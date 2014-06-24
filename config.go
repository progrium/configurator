package main

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"sync"

	"github.com/flynn/go-shlex"
)

type ExecError struct {
	Type   string
	Err    error
	Output string
	Input  string
}

func (e ExecError) Error() string {
	return e.Err.Error()
}

type Executor interface {
	Run() error
	GetEnv() []string
	SetEnv([]string)
	SetStdin(io.Reader)
	SetStdout(io.Writer)
	SetStderr(io.Writer)
}

type OsExecutor struct {
	cmd *exec.Cmd
}

func (o *OsExecutor) Run() error {
	return o.cmd.Run()
}

func (o *OsExecutor) GetEnv() []string {
	return o.cmd.Env
}

func (o *OsExecutor) SetEnv(s []string) {
	o.cmd.Env = s
}

func (o *OsExecutor) SetStdin(r io.Reader) {
	o.cmd.Stdin = r
}

func (o *OsExecutor) SetStdout(w io.Writer) {
	o.cmd.Stdout = w
}

func (o *OsExecutor) SetStderr(w io.Writer) {
	o.cmd.Stderr = w
}

func execCmd(cmdline string) Executor {
	shell := os.Getenv("SHELL")
	if shell != "" {
		return &OsExecutor{cmd: exec.Command(shell, "-c", cmdline)}
	} else {
		cmd, _ := shlex.Split(cmdline) // caught in NewConfig
		if len(cmd) > 1 {
			return &OsExecutor{cmd: exec.Command(cmd[0], cmd[1:]...)}
		} else {
			return &OsExecutor{cmd: exec.Command(cmd[0])}
		}
	}
}

type Config struct {
	sync.Mutex
	execCmd        func(string) Executor
	store          ConfigStore
	tree           *JsonTree
	preprocessor   *Preprocessor
	target         string
	transformCmd   string
	validateCmd    string
	reloadCmd      string
	lastValidBytes []byte
}

func NewConfig(store ConfigStore, target, transform, reload, validate string) (*Config, error) {
	_, err := shlex.Split(transform)
	if err != nil {
		return nil, err
	}
	_, err = shlex.Split(reload)
	if err != nil {
		return nil, err
	}
	_, err = shlex.Split(validate)
	if err != nil {
		return nil, err
	}
	config := &Config{
		store:        store,
		execCmd:      execCmd,
		preprocessor: new(Preprocessor),
		tree:         new(JsonTree),
		target:       target,
		transformCmd: transform,
		reloadCmd:    reload,
		validateCmd:  validate,
	}
	loadBuiltinMacros(config.preprocessor, store, config)
	return config, nil
}

func (c *Config) Load(b []byte) error {
	return c.tree.Load(b)
}

func (c *Config) Dump() []byte {
	return c.tree.Dump()
}

func (c *Config) Get(path string) interface{} {
	return c.tree.GetWrapped(path)
}

func (c *Config) Tree() *JsonTree {
	return c.tree
}

func (c *Config) Mutate(mutation func(*JsonTree) bool) error {
	c.Lock()
	defer c.Unlock()

	cc := c.Copy()
	if err := cc.store.Pull(cc); err != nil {
		return err
	}
	if err := cc.Validate(); err != nil {
		return err
	}

	err := cc.store.Commit(cc, func() error {
		if !mutation(cc.tree) {
			return errors.New("mutation failed")
		}
		if err := cc.Validate(); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	c.replaceTree(cc)
	return c.applyAndReload(cc.lastValidBytes)
}

func (c *Config) Update() error {
	c.Lock()
	defer c.Unlock()

	cc := c.Copy()
	if err := cc.store.Pull(cc); err != nil {
		return err
	}
	output, err := cc.renderAndValidate()
	if err != nil {
		return err
	}

	c.replaceTree(cc)
	err = c.applyAndReload(output)
	if err != nil {
		return err
	}
	return nil
}

func (c *Config) TriggerUpdate(from string) {
	log.Println("config: update triggered by", from)
	err := c.Update()
	if err != nil {
		log.Println("config: update failed, ignoring")
		return
	}
	log.Println("config: update successful")
}

func (c *Config) Validate() error {
	_, err := c.renderAndValidate()
	return err
}

func (c *Config) LastRender() []byte {
	return c.lastValidBytes
}

func (c *Config) Copy() *Config {
	return &Config{
		tree:         c.tree.Copy(),
		preprocessor: c.preprocessor,
		target:       c.target,
		transformCmd: c.transformCmd,
		reloadCmd:    c.reloadCmd,
		validateCmd:  c.validateCmd,
		store:        c.store,
	}
}

func (c *Config) replaceTree(config *Config) {
	c.tree = config.tree
}

func (c *Config) renderAndValidate() ([]byte, error) {
	var output bytes.Buffer
	input := bytes.NewBuffer(c.preprocessor.Process(c.tree).Dump())
	cmd := c.execCmd(c.transformCmd)
	cmd.SetStdin(input)
	cmd.SetStdout(&output)
	if err := cmd.Run(); err != nil {
		return nil, &ExecError{"transform", err, output.String(), input.String()}
	}
	if c.validateCmd != "" {
		if err := c.execValidate(output.Bytes()); err != nil {
			return nil, err
		}
	}
	c.lastValidBytes = output.Bytes()
	return output.Bytes(), nil
}

func (c *Config) execValidate(configBytes []byte) error {
	var output bytes.Buffer
	file, err := ioutil.TempFile(os.TempDir(), "configurator-validate.")
	if err != nil {
		return err
	}
	file.Write(configBytes)
	file.Close()
	cmd := c.execCmd(c.validateCmd)
	cmd.SetEnv(append(cmd.GetEnv(), "FILE="+file.Name()))
	cmd.SetStdout(&output)
	cmd.SetStderr(&output)
	if err := cmd.Run(); err != nil {
		return &ExecError{"validation", err, output.String(), string(configBytes)}
	}
	os.Remove(file.Name())
	return nil
}

func (c *Config) applyAndReload(configBytes []byte) error {
	if err := ioutil.WriteFile(c.target, configBytes, 0644); err != nil {
		return err
	}
	if c.reloadCmd != "" {
		if err := c.execReload(); err != nil {
			return err
		}
	}
	c.lastValidBytes = configBytes
	return nil
}

func (c *Config) execReload() error {
	var output bytes.Buffer
	cmd := c.execCmd(c.reloadCmd)
	cmd.SetStdout(&output)
	cmd.SetStderr(&output)
	if err := cmd.Run(); err != nil {
		return &ExecError{"reload", err, output.String(), ""}
	}
	return nil
}
