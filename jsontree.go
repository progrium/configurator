package main

import (
	"encoding/json"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type JsonTree struct {
	sync.Mutex
	root interface{}
}

func (t *JsonTree) Load(b []byte) error {
	t.Lock()
	defer t.Unlock()
	return json.Unmarshal(b, &t.root)
}

func (t *JsonTree) Dump() []byte {
	bytes, err := json.MarshalIndent(t.Get("/"), "", "  ")
	if err != nil {
		log.Println("jsontree:", err)
	}
	return bytes
}

func (t *JsonTree) Get(path string) interface{} {
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
	parent := t.Get(filepath.Dir(path))
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
			return true
		}
		return false
	}
}

func (t *JsonTree) GetWrapped(path string) interface{} {
	selection := t.Get(path)
	if t.IsComposite(path) {
		return selection
	} else {
		return []interface{}{selection}
	}
}

func (t *JsonTree) IsObject(path string) bool {
	selection := t.Get(path)
	if selection == nil {
		return false
	}
	_, ok := selection.(map[string]interface{})
	return ok
}

func (t *JsonTree) IsArray(path string) bool {
	selection := t.Get(path)
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
	selection, ok := t.Get(path).(map[string]interface{})
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
	selection, ok := t.Get(path).([]interface{})
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
		parent := t.Get(parentPath).(map[string]interface{})
		t.Lock()
		defer t.Unlock()
		delete(parent, key)
		return true
	} else {
		parent := t.Get(parentPath).([]interface{})
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
	err := tree.Load(t.Dump())
	assert(err)
	return tree
}

func (t *JsonTree) Paths() []string {
	paths := []string{}
	var walk func(string)
	walk = func(path string) {
		paths = append(paths, path)
		if t.IsObject(path) {
			obj := t.Get(path).(map[string]interface{})
			path = strings.TrimSuffix(path, "/")
			for k, _ := range obj {
				walk(path + "/" + k)
			}
		} else if t.IsArray(path) {
			arr := t.Get(path).([]interface{})
			path = strings.TrimSuffix(path, "/")
			for i, _ := range arr {
				walk(path + "/" + strconv.Itoa(i))
			}
		}
	}
	walk("/")
	return paths
}
