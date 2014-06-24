package main

import "io"

type DummyExecutor struct{}

func (d *DummyExecutor) Run() error {
	return nil
}

func (d *DummyExecutor) GetEnv() []string {
	return make([]string, 0)
}

func (d *DummyExecutor) SetEnv(s []string) {
}

func (d *DummyExecutor) SetStdin(r io.Reader) {
}

func (d *DummyExecutor) SetStdout(w io.Writer) {
}

func (d *DummyExecutor) SetStderr(w io.Writer) {
}

func testCmd(cmdline string) Executor {
	return &DummyExecutor{}
}
