package main

import (
	"bytes"
	"errors"
	"log"
	"net/url"
	"sync"

	"github.com/armon/consul-kv"
)

type ConsulStore struct {
	sync.Mutex
	client    *consulkv.Client
	prefix    string
	lastIndex uint64
	watching  map[string]struct{}
}

func NewConsulStore(uri *url.URL) (*ConsulStore, error) {
	client, err := consulkv.NewClient(consulkv.DefaultConfig())
	if err != nil {
		return nil, err
	}
	return &ConsulStore{
		client:   client,
		prefix:   uri.Path,
		watching: make(map[string]struct{}),
	}, nil
}

func (s *ConsulStore) Watch(config *Config, key string) {
	s.Lock()
	_, watching := s.watching[key]
	if watching {
		s.Unlock()
		return
	}
	s.watching[key] = struct{}{}
	index := s.lastIndex
	s.Unlock()
	for {
		_, pair, err := s.client.WatchGet(key, index)
		if err != nil {
			log.Println("consul:", err)
		}
		if pair.ModifyIndex == index {
			// watch was released after timeout
			continue
		}
		go func() {
			log.Println("consul: update triggered")
			err := config.Update()
			if err != nil {
				log.Println("consul: update failed, ignoring")
				return
			}
			log.Println("consul: update successful")
		}()
		index = pair.ModifyIndex
	}
}

func (s *ConsulStore) Pull(config *Config) (bool, error) {
	s.Lock()
	defer s.Unlock()
	configPath := s.prefix + "/config"
	go s.Watch(config, configPath) // actually runs on exit due to lock
	currentDump := config.Dump()
	meta, pair, err := s.client.Get(configPath)
	if err != nil {
		return false, err
	}
	if pair != nil {
		err := config.Load(pair.Value)
		if err != nil {
			log.Println("Invalid JSON from config store value", configPath)
			return false, err
		}
		s.lastIndex = meta.ModifyIndex
	}
	return !bytes.Equal(config.Dump(), currentDump), nil
}

func (s *ConsulStore) Commit(config *Config, operation func() error) error {
	var err error
	var success bool
	var tries int
	for !success && tries < 3 {
		tries++
		if err := operation(); err != nil {
			return err
		}
		s.Lock()
		success, err = s.client.CAS(s.prefix+"/config", config.Dump(), 0, s.lastIndex)
		s.Unlock()
		if err != nil {
			return err
		}
		if success {
			return nil
		}
		_, err = s.Pull(config)
		if err != nil {
			return err
		}
	}
	return errors.New("unable to commit after 3 tries")
}

func (s *ConsulStore) Preprocess(tree *JsonTree) *JsonTree {
	newtree := tree.Copy()
	/*for path, d := range newtree.Directives() {
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
	}*/
	return newtree
}
