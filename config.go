package main

import (
	"bytes"
	"errors"
	"io/ioutil"
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

func execCmd(cmdline string) *exec.Cmd {
	shell := os.Getenv("SHELL")
	if shell != "" {
		return exec.Command(shell, "-c", cmdline)
	} else {
		cmd, _ := shlex.Split(cmdline) // caught in NewConfig
		if len(cmd) > 1 {
			return exec.Command(cmd[0], cmd[1:]...)
		} else {
			return exec.Command(cmd[0])
		}
	}
}

type Config struct {
	sync.Mutex
	store          *ConsulStore
	tree           *JsonTree
	target         string
	transformCmd   string
	validateCmd    string
	reloadCmd      string
	lastValidBytes []byte
}

func NewConfig(store *ConsulStore, target, transform, reload, validate string) (*Config, error) {
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
	return &Config{
		store:        store,
		tree:         new(JsonTree),
		target:       target,
		transformCmd: transform,
		reloadCmd:    reload,
		validateCmd:  validate,
	}, nil
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

	cc := c.copy()
	_, err := cc.store.Pull(cc)
	if err != nil {
		return err
	}
	if err := cc.Validate(); err != nil {
		return err
	}

	err = cc.store.Commit(cc, func() error {
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

	cc := c.copy()
	updated, err := cc.store.Pull(cc)
	if err != nil {
		return err
	}
	if !updated {
		return nil
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

func (c *Config) Validate() error {
	_, err := c.renderAndValidate()
	return err
}

func (c *Config) LastRender() []byte {
	return c.lastValidBytes
}

func (c *Config) copy() *Config {
	return &Config{
		tree:         c.tree.Copy(),
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
	input := bytes.NewBuffer(c.store.Preprocess(c.tree).Dump())
	cmd := execCmd(c.transformCmd)
	cmd.Stdin = input
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return nil, &ExecError{"transform", err, output.String(), input.String()}
	}

	if len(c.validateCmd) > 0 {
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
	defer os.Remove(file.Name())
	file.Write(configBytes)
	file.Close()
	cmd := execCmd(c.validateCmd)
	cmd.Env = append(cmd.Env, "FILE="+file.Name())
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return &ExecError{"validation", err, output.String(), string(configBytes)}
	}
	return nil
}

func (c *Config) applyAndReload(configBytes []byte) error {
	if err := ioutil.WriteFile(c.target, configBytes, 0644); err != nil {
		return err
	}

	if len(c.reloadCmd) > 0 {
		if err := c.execReload(); err != nil {
			return err
		}
	}

	c.lastValidBytes = configBytes
	return nil
}

func (c *Config) execReload() error {
	var output bytes.Buffer
	cmd := execCmd(c.reloadCmd)
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return &ExecError{"reload", err, output.String(), ""}
	}
	return nil
}
