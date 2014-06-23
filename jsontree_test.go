package main

import (
	"reflect"

	"testing"
)

const (
	jsoninput = `{"test": 3,"othertest":1,"void":{"json":3},"array":[1,3,5,2]}`
)

func makejsontree(t *testing.T) *JsonTree {
	tree := &JsonTree{}

	input := []byte(jsoninput)
	err := tree.Load(input)
	if err != nil {
		t.Fatalf("failed to load standard input, %v", err)
	}

	return tree
}

func TestJsonLoad(t *testing.T) {
	tree := makejsontree(t)

	if n := tree.Get("test").(float64); n != 3 {
		t.Fatalf("test is wrong: %v", n)
	}

	if n := tree.Get("othertest").(float64); n != 1 {
		t.Fatalf("othertest is wrong: %v", n)
	}

	subtree := tree.Get("void").(map[string]interface{})

	if n := subtree["json"].(float64); n != 3 {
		t.Fatalf("void/json subtree is wrong: %v", n)
	}

	if n := tree.Get("void/json").(float64); n != 3 {
		t.Fatalf("void/json direct is wrong: %v", n)
	}

	if n := tree.Get("void/non_existing"); n != nil {
		t.Fatalf("void/non_existing apparently exists: %v", n)
	}
}

func TestJsonDump(t *testing.T) {
	tree := makejsontree(t)

	result := string(tree.Dump())
	supposed := `{
  "array": [
    1,
    3,
    5,
    2
  ],
  "othertest": 1,
  "test": 3,
  "void": {
    "json": 3
  }
}`
	if result != supposed {
		t.Fatalf("dump failed: %v", result)
	}
}

func TestJsonWrapped(t *testing.T) {
	tree := makejsontree(t)

	// Test if it gets wrapped
	if a := tree.GetWrapped("void/json"); reflect.TypeOf(a).Kind() != reflect.Slice {
		t.Fatalf("wrapping void/json failed: %v", a)
	}

	// While this shouldn't get wrapped
	if a := tree.GetWrapped("void"); reflect.TypeOf(a).Kind() != reflect.Map {
		t.Fatalf("wrapping void failed: %v", a)
	}

	// Nor should this
	if a := tree.GetWrapped("array"); reflect.TypeOf(a).Kind() != reflect.Slice {
		t.Fatalf("wrapping array failed: %v", a)
	}
}

func TestJsonMerge(t *testing.T) {
	tree := makejsontree(t)

	addnew := make(map[string]interface{})
	addnew["213"] = "323"
	addnew["test2"] = 49

	attempt := tree.Merge("new", addnew)
	if attempt {
		t.Fatal("shouldn't have merged")
	}

	attempt = tree.Merge("/", addnew)
	if !attempt {
		t.Fatal("failed to merge")
	}

	result := string(tree.Dump())
	supposed := `{
  "213": "323",
  "array": [
    1,
    3,
    5,
    2
  ],
  "othertest": 1,
  "test": 3,
  "test2": 49,
  "void": {
    "json": 3
  }
}`

	if result != supposed {
		t.Fatalf("dump failed: %v", result)
	}
}

func TestJsonAppend(t *testing.T) {
	tree := makejsontree(t)

	try := tree.Append("/array", 15)
	if !try {
		t.Fatal("failed to append")
	}

	result := string(tree.Dump())
	supposed := `{
  "array": [
    1,
    3,
    5,
    2,
    15
  ],
  "othertest": 1,
  "test": 3,
  "void": {
    "json": 3
  }
}`

	if result != supposed {
		t.Fatalf("dump failed: %v", result)
	}
}

func TestJsonCopy(t *testing.T) {
	tree := makejsontree(t)

	copy := tree.Copy()

	if cpyStr := string(copy.Dump()); string(tree.Dump()) != cpyStr {
		t.Fatalf("dump is not the same: %v", cpyStr)
	}
}

func TestJsonDelete(t *testing.T) {
	tree := makejsontree(t)

	if !tree.Delete("/array") {
		t.Fatal("failed to delete /array")
	}

	if res := tree.Get("/array"); res != nil {
		t.Fatalf("should be nil, %v", res)
	}
}

func TestJsonReplace(t *testing.T) {
	tree := makejsontree(t)

	if !tree.Replace("/array/0", 4) {
		t.Fatal("failed to first element with 4")
	}

	if elem := tree.Get("/array/0").(int); elem != 4 {
		t.Fatalf("first element is wrong, %v", elem)
	}

	if !tree.Replace("/array", make([]interface{}, 0)) {
		t.Fatal("failed to replace with empty array")
	}

	if arr := tree.Get("/array").([]interface{}); len(arr) != 0 {
		t.Fatalf("/array is not len 0, %v", arr)
	}

	if !tree.Replace("/void/new", 1) {
		t.Fatal("failed to add new element void/new")
	}

	if res := tree.Get("/void/new").(int); res != 1 {
		t.Fatalf("/void/new returned wrong value, %v", res)
	}

}
