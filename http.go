package main

import (
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

func runHttp(config *Config) {
	http.HandleFunc("/v1/render", func(w http.ResponseWriter, req *http.Request) {
		log.Println(req.Method, req.RequestURI)
		switch req.Method {
		case "GET":
			w.Write(config.LastRender())
		case "POST":
			body, err := ioutil.ReadAll(req.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, "Bad request: "+err.Error())
				return
			}
			newconfig := config.Copy()
			err = newconfig.Load(body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, "Bad request: "+err.Error())
				return
			}
			err = newconfig.Validate()
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				if e, ok := err.(*ExecError); ok {
					io.WriteString(w, e.Output)
					io.WriteString(w, e.Input)
				} else {
					io.WriteString(w, err.Error())
				}
				return
			}
			w.Write(newconfig.LastRender())

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/v1/config/", func(w http.ResponseWriter, req *http.Request) {
		log.Println(req.Method, req.RequestURI)
		path := strings.TrimPrefix(req.RequestURI, "/v1/config")
		handleMutateError := func(err error) {
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusBadRequest)
				if e, ok := err.(*ExecError); ok {
					io.WriteString(w, e.Output)
				} else {
					io.WriteString(w, err.Error())
				}
			}
		}
		readJsonBody := func() interface{} {
			var json interface{}
			if err := unmarshal(req.Body, &json); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, "Bad request: "+err.Error())
				return nil
			}
			return json
		}
		switch req.Method {
		case "GET":
			w.Header().Add("Content-Type", "application/json")
			w.Write(append(marshal(config.Get(path)), '\n'))
		case "POST":
			if !config.Tree().IsComposite(path) {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			json := readJsonBody()
			if json == nil {
				return
			}
			obj, isObj := json.(map[string]interface{})
			err := config.Mutate(func(c *JsonTree) bool {
				if c.IsObject(path) && isObj {
					return c.Merge(path, obj)
				} else {
					return c.Append(path, json)
				}
			})
			handleMutateError(err)
		case "PUT":
			json := readJsonBody()
			if json == nil {
				return
			}
			err := config.Mutate(func(c *JsonTree) bool {
				return c.Replace(path, json)
			})
			handleMutateError(err)
		case "DELETE":
			err := config.Mutate(func(c *JsonTree) bool {
				return c.Delete(path)
			})
			handleMutateError(err)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	log.Println("Listening on port " + *port)
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}
