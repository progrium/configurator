package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
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

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %v [options] <configstore-uri> <transformer> <target-file>\n\n", os.Args[0])
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()
	if flag.NArg() < 3 {
		flag.Usage()
		os.Exit(64)
	}

	uri, err := url.Parse(flag.Arg(0))
	assert(err)
	factory := map[string]func(*url.URL) (*ConsulStore, error){
		"consul": NewConsulStore,
	}[uri.Scheme]
	if factory == nil {
		log.Fatal("Unrecognized config store backend: ", uri.Scheme)
	}

	store, err := factory(uri)
	assert(err)

	transformer := flag.Arg(1)
	target := flag.Arg(2)

	config, err := NewConfig(store, target, transformer, *reloadCmd, *checkCmd)
	assert(err)

	log.Printf("Pulling and validating from %s...\n", flag.Arg(0))
	err = config.Update()
	if e, ok := err.(*ExecError); ok {
		fmt.Printf("!! Initial pull from config store resulted in validation error.\n")
		fmt.Printf("!! Output of '%s':\n", *checkCmd)
		fmt.Println(e.Output)
		os.Exit(3)
	} else {
		assert(err)
	}

	runHttp(config)
}
