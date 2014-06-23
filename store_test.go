package main

type TestStore struct{}

func NewTestStore() *TestStore {
	return &TestStore{}
}

func (s *TestStore) Get(key string) string {
	switch key {
	case "/one":
		return `1`
	case "/two":
		return `2`
	case "/three":
		return `3`
	}
	return ""
}

func (s *TestStore) WatchToUpdate(config *Config, key string) {
	// nope.
}

func (s *TestStore) Pull(config *Config) error {
	return nil
}

func (s *TestStore) Commit(config *Config, operation func() error) error {
	return nil
}
