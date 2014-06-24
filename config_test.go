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

func (d *DummyExecutor) SetStdio(in io.Reader, out io.Writer, err io.Writer) {
}

func testCmd(cmdline string) Executor {
	return &DummyExecutor{}
}
