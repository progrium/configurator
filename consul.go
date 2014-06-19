package main

import (
	"errors"
	"log"
	"net/url"
	"sync"

	"github.com/armon/consul-kv"
)

type ConsulStore struct {
	sync.Mutex
	client      *consulkv.Client
	prefix      string
	configIndex uint64
	watching    map[string]struct{}
}

func NewConsulStore(uri *url.URL) (ConfigStore, error) {
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

func (s *ConsulStore) Get(key string) string {
	s.Lock()
	defer s.Unlock()
	_, pair, err := s.client.Get(key)
	if err != nil {
		log.Println("consul:", err)
	}
	if pair != nil {
		return string(pair.Value)
	}
	return ""
}

func (s *ConsulStore) WatchToUpdate(config *Config, key string) {
	s.Lock()
	_, watching := s.watching[key]
	if watching {
		s.Unlock()
		return
	}
	s.watching[key] = struct{}{}
	var index uint64
	s.Unlock()
	for {
		_, pair, err := s.client.WatchGet(key, index)
		if err != nil {
			log.Println("consul:", err)
			return
		}
		if pair == nil {
			log.Println("consul: key does not exist, so cannot watch", key)
			return
		}
		if pair.ModifyIndex == index {
			// watch was released after timeout
			continue
		}
		if index != 0 {
			go config.TriggerUpdate(key)
		}
		index = pair.ModifyIndex
	}
}

func (s *ConsulStore) Pull(config *Config) error {
	s.Lock()
	defer s.Unlock()
	configPath := s.prefix + "/config"
	go s.WatchToUpdate(config, configPath) // actually runs on exit due to lock
	meta, pair, err := s.client.Get(configPath)
	if err != nil {
		return err
	}
	if pair != nil {
		err := config.Load(pair.Value)
		if err != nil {
			log.Println("consul: Invalid JSON from config store value", configPath)
			return err
		}
		s.configIndex = meta.ModifyIndex
	}
	return nil
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
		success, err = s.client.CAS(s.prefix+"/config", config.Dump(), 0, s.configIndex)
		s.Unlock()
		if err != nil {
			return err
		}
		if success {
			return nil
		}
		if err := s.Pull(config); err != nil {
			return err
		}
	}
	return errors.New("consul: unable to commit after 3 tries")
}
