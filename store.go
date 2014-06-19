package main

import (
	"io/ioutil"
	"log"
	"net/url"
	"os"
)

type ConfigStore interface {
	Get(key string) string
	WatchToUpdate(config *Config, key string)
	Pull(config *Config) error
	Commit(config *Config, operation func() error) error
}

type FileStore struct {
	path string
}

func NewFileStore(uri *url.URL) (ConfigStore, error) {
	if _, err := os.Stat(uri.Path); os.IsNotExist(err) {
		return nil, err
	}
	return &FileStore{
		path: uri.Path,
	}, nil
}

func (s *FileStore) Get(key string) string {
	bytes, err := ioutil.ReadFile(key)
	if err != nil {
		log.Println("filestore:", err)
	}
	if len(bytes) > 0 {
		return string(bytes)
	}
	return ""
}

func (s *FileStore) WatchToUpdate(config *Config, key string) {
	// twiddle our thumbs. we're not going to watch file changes. yet?
}

func (s *FileStore) Pull(config *Config) error {
	configData := s.Get(s.path)
	if configData != "" {
		err := config.Load([]byte(configData))
		if err != nil {
			log.Println("filestore: Invalid JSON from config store file", s.path)
			return err
		}
	}
	return nil
}

func (s *FileStore) Commit(config *Config, operation func() error) error {
	if err := operation(); err != nil {
		return err
	}
	if err := ioutil.WriteFile(s.path, config.Dump(), 0644); err != nil {
		return err
	}
	return nil
}
