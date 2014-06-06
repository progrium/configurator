package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/armon/consul-kv"
	"github.com/flynn/go-shlex"
)

var port = flag.String("p", "8881", "port to listen on")
var checkCmd = flag.String("c", "", "config check command. FILE set in env")
var reloadCmd = flag.String("r", "", "reload command")

func assert(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func marshal(obj interface{}) []byte {
	bytes, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		log.Println("marshal:", err)
	}
	return bytes
}

func unmarshal(input io.ReadCloser, obj interface{}) error {
	body, err := ioutil.ReadAll(input)
	if err != nil {
		return err
	}
	err = json.Unmarshal(body, obj)
	if err != nil {
		return err
	}
	return nil
}

func unpack(obj interface{}) (map[string]interface{}, []interface{}, interface{}) {
	switch unpacked := obj.(type) {
	case map[string]interface{}:
		return unpacked, nil, nil
	case []interface{}:
		return nil, unpacked, nil
	default:
		return nil, nil, unpacked
	}
}

type JsonTree struct {
	sync.Mutex
	root interface{}
}

var Directives = []string{"$file", "$value"}

func (t *JsonTree) Load(b []byte) error {
	t.Lock()
	defer t.Unlock()
	return json.Unmarshal(b, &t.root)
}

func (t *JsonTree) get(path string) interface{} {
	t.Lock()
	defer t.Unlock()
	if path == "/" {
		return t.root
	}
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	var selection interface{}
	selection = t.root
	for _, key := range parts {
		switch selection.(type) {
		case map[string]interface{}:
			selection = selection.(map[string]interface{})[key]
		case []interface{}:
			i, err := strconv.Atoi(key)
			if err != nil {
				return nil
			}
			if i >= len(selection.([]interface{})) {
				return nil
			}
			selection = selection.([]interface{})[i]
		default:
			selection = nil
		}
	}
	return selection
}

func (t *JsonTree) setter(path string) func(obj interface{}) bool {
	if path == "/" {
		return func(obj interface{}) bool {
			t.Lock()
			defer t.Unlock()
			t.root = obj
			return true
		}
	}
	parent := t.get(filepath.Dir(path))
	key := filepath.Base(path)
	return func(obj interface{}) bool {
		t.Lock()
		defer t.Unlock()
		switch p := parent.(type) {
		case map[string]interface{}:
			p[key] = obj
			return true
		case []interface{}:
			i, err := strconv.Atoi(key)
			if err != nil {
				return false
			}
			if i >= len(p) {
				return false
			}
			p[i] = obj
		}
		return false
	}
}

func (t *JsonTree) Get(path string) interface{} {
	selection := t.get(path)
	if t.IsComposite(path) {
		return selection
	} else {
		return []interface{}{selection}
	}
}

func (t *JsonTree) IsObject(path string) bool {
	selection := t.get(path)
	if selection == nil {
		return false
	}
	_, ok := selection.(map[string]interface{})
	return ok
}

func (t *JsonTree) IsArray(path string) bool {
	selection := t.get(path)
	if selection == nil {
		return false
	}
	_, ok := selection.([]interface{})
	return ok
}

func (t *JsonTree) IsComposite(path string) bool {
	return t.IsArray(path) || t.IsObject(path)
}

func (t *JsonTree) Merge(path string, obj map[string]interface{}) bool {
	selection, ok := t.get(path).(map[string]interface{})
	if !ok {
		return false
	}
	t.Lock()
	defer t.Unlock()
	for k, v := range obj {
		selection[k] = v
	}
	return true
}

func (t *JsonTree) Append(path string, obj interface{}) bool {
	selection, ok := t.get(path).([]interface{})
	if !ok {
		return false
	}
	setter := t.setter(path)
	return setter(append(selection, obj))
}

func (t *JsonTree) Replace(path string, obj interface{}) bool {
	return t.setter(path)(obj)
}

func (t *JsonTree) Delete(path string) bool {
	parentPath := filepath.Dir(path)
	key := filepath.Base(path)
	if t.IsObject(parentPath) {
		parent := t.get(parentPath).(map[string]interface{})
		t.Lock()
		defer t.Unlock()
		delete(parent, key)
		return true
	} else {
		parent := t.get(parentPath).([]interface{})
		i, err := strconv.Atoi(key)
		if err != nil {
			return false
		}
		if i >= len(parent) {
			return false
		}
		return t.setter(parentPath)(append(parent[:i], parent[i+1:]...))
	}
}

func (t *JsonTree) Copy() *JsonTree {
	tree := new(JsonTree)
	err := tree.Load(marshal(t.get("/")))
	assert(err)
	return tree
}

func (t *JsonTree) Paths() []string {
	paths := []string{}
	var walk func(string)
	walk = func(path string) {
		paths = append(paths, path)
		if t.IsObject(path) {
			obj := t.get(path).(map[string]interface{})
			path = strings.TrimSuffix(path, "/")
			for k, _ := range obj {
				walk(path + "/" + k)
			}
		} else if t.IsArray(path) {
			arr := t.get(path).([]interface{})
			path = strings.TrimSuffix(path, "/")
			for i, _ := range arr {
				walk(path + "/" + strconv.Itoa(i))
			}
		}
	}
	walk("/")
	return paths
}

func (t *JsonTree) Directives() map[string]map[string]string {
	directives := make(map[string]map[string]string)
	for _, path := range t.Paths() {
		for _, directive := range Directives {
			if strings.HasSuffix(path, directive) {
				obj := t.get(filepath.Dir(path)).(map[string]interface{})
				obj2 := make(map[string]string)
				for k, v := range obj {
					obj2[k] = v.(string)
				}
				directives[filepath.Dir(path)] = obj2
			}
		}
	}
	return directives
}

type ConsulStore struct {
	sync.Mutex
	client    *consulkv.Client
	prefix    string
	lastIndex uint64
}

func NewConsulStore(prefix string) (*ConsulStore, error) {
	client, err := consulkv.NewClient(consulkv.DefaultConfig())
	if err != nil {
		return nil, err
	}
	return &ConsulStore{
		client: client,
		prefix: prefix,
	}, nil
}

func (s *ConsulStore) Pull(tree *JsonTree) error {
	s.Lock()
	defer s.Unlock()
	meta, pair, err := s.client.Get(s.prefix + "/config")
	if err != nil {
		return err
	}
	if pair != nil {
		err := tree.Load(pair.Value)
		if err != nil {
			return err
		}
		s.lastIndex = meta.ModifyIndex
	}
	return nil
}

func (s *ConsulStore) Commit(tree *JsonTree, operation func(*JsonTree)) error {
	var err error
	var success bool
	var tries int
	for !success && tries < 3 {
		tries++
		operation(tree)
		s.Lock()
		success, err = s.client.CAS(s.prefix+"/config", marshal(tree.Get("/")), 0, s.lastIndex)
		s.Unlock()
		if err != nil {
			return err
		}
		if success {
			return nil
		}
		err = s.Pull(tree)
		if err != nil {
			return err
		}
	}
	return errors.New("unable to commit after 3 tries")
}

func (s *ConsulStore) Preprocess(tree *JsonTree) *JsonTree {
	newtree := tree.Copy()
	for path, d := range newtree.Directives() {
		if d["$value"] != "" {
			_, pair, err := s.client.Get(d["$value"])
			if err != nil {
				log.Println("preprocess:", err)
			}
			if pair != nil {
				newtree.Replace(path, string(pair.Value))
			} else {
				newtree.Replace(path, nil)
			}
		}
	}
	return newtree
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %v [options] <config-prefix> <transformer> <target-file>\n\n", os.Args[0])
		flag.PrintDefaults()
	}
}

func NewRenderer(transformer, target string, store *ConsulStore, tree *JsonTree) func() bool {
	return func() bool {
		var output bytes.Buffer
		input := bytes.NewBuffer(
			marshal(store.Preprocess(tree).Get("/")))
		transformCmd, err := shlex.Split(transformer)
		if err != nil {
			log.Println(err)
			return false
		}
		cmd := exec.Command(transformCmd[0], transformCmd[1:]...)
		cmd.Stdin = input
		cmd.Stdout = &output
		if err := cmd.Run(); err != nil {
			log.Println(err)
			log.Println(output.String())
			return false
		}

		if *checkCmd != "" {
			file, err := ioutil.TempFile(os.TempDir(), "staging")
			if err != nil {
				log.Println(err)
				return false
			}
			defer os.Remove(file.Name())
			file.Write(output.Bytes())
			file.Close()
			checkCmdline, err := shlex.Split(*checkCmd)
			if err != nil {
				log.Println(err)
				return false
			}
			cmd := exec.Command(checkCmdline[0], checkCmdline[1:]...)
			cmd.Env = append(cmd.Env, "FILE="+file.Name())
			var checkOutput bytes.Buffer
			cmd.Stdout = &checkOutput
			if err := cmd.Run(); err != nil {
				log.Println(err)
				log.Println(checkOutput.String())
				return false
			}
		}

		err = ioutil.WriteFile(target, output.Bytes(), 0644)
		if err != nil {
			log.Println(err)
			return false
		}

		if *reloadCmd != "" {
			reloadCmdline, err := shlex.Split(*reloadCmd)
			if err != nil {
				log.Println(err)
				return false
			}
			cmd := exec.Command(reloadCmdline[0], reloadCmdline[1:]...)
			var reloadOutput bytes.Buffer
			cmd.Stdout = &reloadOutput
			if err := cmd.Run(); err != nil {
				log.Println(err)
				log.Println(reloadOutput.String())
				return false
			}
		}

		return true
	}
}

func main() {
	flag.Parse()
	if flag.NArg() < 3 {
		flag.Usage()
		os.Exit(64)
	}

	configPrefix := flag.Arg(0)
	transformer := flag.Arg(1)
	targetFile := flag.Arg(2)

	config := new(JsonTree)
	store, err := NewConsulStore(configPrefix)
	assert(err)

	assert(store.Pull(config))

	renderer := NewRenderer(transformer, targetFile, store, config)

	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		log.Println(req.Method, req.RequestURI)
		switch req.Method {
		case "GET":
			store.Pull(config)
			w.Header().Add("Content-Type", "application/json")
			w.Write(append(marshal(config.Get(req.RequestURI)), '\n'))
			renderer()
		case "POST":
			store.Pull(config)
			if !config.IsComposite(req.RequestURI) {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var json interface{}
			if err := unmarshal(req.Body, &json); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Bad request: " + err.Error()))
				return
			}
			obj, _, _ := unpack(json)
			err := store.Commit(config, func(c *JsonTree) {
				if c.IsObject(req.RequestURI) && obj != nil {
					c.Merge(req.RequestURI, obj)
				} else {
					c.Append(req.RequestURI, json)
				}
			})
			if err != nil {
				log.Println(err)
			}
			renderer()
		case "PUT":
			store.Pull(config)
			var json interface{}
			if err := unmarshal(req.Body, &json); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Bad request: " + err.Error()))
				return
			}
			err := store.Commit(config, func(c *JsonTree) {
				c.Replace(req.RequestURI, json)
			})
			if err != nil {
				log.Println(err)
			}
			renderer()
		case "DELETE":
			store.Pull(config)
			err := store.Commit(config, func(c *JsonTree) {
				c.Delete(req.RequestURI)
			})
			if err != nil {
				log.Println(err)
			}
			renderer()
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	log.Println("Listening on port " + *port)
	log.Fatal(http.ListenAndServe(":"+*port, nil))

}
