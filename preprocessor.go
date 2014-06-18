package main

import (
	"path/filepath"
	"sync"
)

type macroinput map[string]interface{}
type macrofn func(macroinput) interface{}

type Preprocessor struct {
	sync.Mutex
	macros map[string]macrofn
	ready  bool
}

func (p *Preprocessor) Register(name string, fn macrofn) {
	p.Lock()
	defer p.Unlock()
	if !p.ready {
		p.macros = make(map[string]macrofn)
		p.ready = true
	}
	p.macros[name] = fn
}

func (p *Preprocessor) Process(tree *JsonTree) *JsonTree {
	p.Lock()
	defer p.Unlock()
	newtree := tree.Copy()
	for _, namepath := range p.macroPaths(newtree) {
		name := filepath.Base(namepath)
		path := filepath.Dir(namepath)
		input := macroinput(tree.Get(path).(map[string]interface{}))
		newtree.Replace(path, p.macros[name](input))
	}
	return newtree
}

func (p *Preprocessor) macroPaths(t *JsonTree) []string {
	paths := make([]string, 0)
	for _, path := range t.Paths() {
		for name, _ := range p.macros {
			if filepath.Base(path) == name {
				paths = append(paths, path)
			}
		}
	}
	return paths
}
