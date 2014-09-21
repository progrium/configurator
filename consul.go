package main

import (
	"errors"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/armon/consul-api"
)

type ConsulStore struct {
	sync.Mutex
	client      *consulapi.Client
	prefix      string
	configIndex uint64
	watching    map[string]struct{}
}

func NewConsulStore(uri *url.URL) (ConfigStore, error) {
	config := consulapi.DefaultConfig()
	if uri.Host != "" {
		config.Address = uri.Host
	}
	client, err := consulapi.NewClient(config)
	if err != nil {
		return nil, err
	}
	return &ConsulStore{
		client:   client,
		prefix:   uri.Path[1:],
		watching: make(map[string]struct{}),
	}, nil
}

func (s *ConsulStore) Get(key string) string {
	s.Lock()
	defer s.Unlock()
	pair, _, err := s.client.KV().Get(key, nil)
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
		pair, _, err := s.client.KV().Get(key, &consulapi.QueryOptions{
			WaitTime:  time.Duration(10) * time.Minute,
			WaitIndex: index,
		})
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
	pair, _, err := s.client.KV().Get(configPath, nil)
	if err != nil {
		log.Println("consul: pull:", err)
		return err
	}
	if pair != nil {
		err := config.Load(pair.Value)
		if err != nil {
			log.Println("consul: Invalid JSON from config store value", configPath)
			return err
		}
		s.configIndex = pair.ModifyIndex
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
		success, _, err = s.client.KV().CAS(&consulapi.KVPair{
			Key:         s.prefix + "/config",
			Value:       config.Dump(),
			ModifyIndex: s.configIndex,
		}, nil)
		s.Unlock()
		if err != nil {
			log.Println("consul: commit:", err)
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
