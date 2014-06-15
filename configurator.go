package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
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

func unpack(obj interface{}) (map[string]interface{}, []interface{}, interface{}) {
	switch unpacked := obj.(type) {
	case map[string]interface{}:
		return unpacked, nil, nil
	case []interface{}:
		return nil, unpacked, nil
	default:
		return nil, nil, unpacked
	}
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %v [options] <config-prefix> <transformer> <target-file>\n\n", os.Args[0])
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()
	if flag.NArg() < 3 {
		flag.Usage()
		os.Exit(64)
	}

	configPrefix := flag.Arg(0)
	transformer := flag.Arg(1)
	target := flag.Arg(2)

	store, err := NewConsulStore(configPrefix)
	assert(err)

	config, err := NewConfig(store, target, transformer, *reloadCmd, *checkCmd)
	assert(err)

	err = config.Update()
	if e, ok := err.(*ExecError); ok {
		fmt.Printf("!! Initial pull from config store resulted in validation error.\n")
		fmt.Printf("!! Output of '%s':\n", *checkCmd)
		fmt.Println(e.Output)
		os.Exit(3)
	} else {
		assert(err)
	}

	http.HandleFunc("/v1/render", func(w http.ResponseWriter, req *http.Request) {
		// GET: show last applied render
		// POST: render for given input (TODO)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		w.Write(config.LastRender())
	})

	http.HandleFunc("/v1/config/", func(w http.ResponseWriter, req *http.Request) {
		log.Println(req.Method, req.RequestURI)
		path := strings.TrimPrefix(req.RequestURI, "/v1/config")
		switch req.Method {
		case "GET":
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error()))
				return
			}
			w.Header().Add("Content-Type", "application/json")
			w.Write(append(marshal(config.Get(path)), '\n'))
		case "POST":
			if !config.Tree().IsComposite(path) {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var json interface{}
			if err := unmarshal(req.Body, &json); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, "Bad request: "+err.Error())
				return
			}
			obj, _, _ := unpack(json)
			err := config.Mutate(func(c *JsonTree) bool {
				if c.IsObject(path) && obj != nil {
					return c.Merge(path, obj)
				} else {
					return c.Append(path, json)
				}
			})
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusBadRequest)
				if e, ok := err.(*ExecError); ok {
					io.WriteString(w, e.Output)
				} else {
					io.WriteString(w, err.Error())
				}
			}
		case "PUT":
			var json interface{}
			if err := unmarshal(req.Body, &json); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, "Bad request: "+err.Error())
				return
			}
			err := config.Mutate(func(c *JsonTree) bool {
				return c.Replace(path, json)
			})
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusBadRequest)
				if e, ok := err.(*ExecError); ok {
					io.WriteString(w, e.Output)
				} else {
					io.WriteString(w, err.Error())
				}
			}
		case "DELETE":
			err := config.Mutate(func(c *JsonTree) bool {
				return c.Delete(path)
			})
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusBadRequest)
				if e, ok := err.(*ExecError); ok {
					io.WriteString(w, e.Output)
				} else {
					io.WriteString(w, err.Error())
				}
			}
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	log.Println("Listening on port " + *port)
	log.Fatal(http.ListenAndServe(":"+*port, nil))

}
