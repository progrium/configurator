{
	"$for": "@service", 
	"in": {"$services": "*"},
	"loop": ["upstream", {"@service": null}],
	"else": ["noupstream"]
}

{
	"$if": {
		"$eq": [
			{"$environ": "FOOBAR"}, 
			"foobar"
		]
	},
 	"then": "Blah",
 	"else": "Other"
 }
{"$environ": "FOOBAR"}
{"$service": "myapp", ".": "Port"}
{"$ref": "#foo/bar"}
{"$set": ["@foo", "value"]}


remote: config store change notification (config or reference value)
local: http mutation, config store pull

any change should be validated before commit (going out) or apply (coming in)

incoming(remote mutation): copy, validate(change, render), replace, apply
outgoing(local mutation): copy, pull(copy, change, render), validate, commit(write, [pull, validate, write, ...])

for {
	select {
	case <-foo:
		go trigger Update
	}
}


any change should go through render before apply
keep around last rendered


TODO
 * macro system
 * abstract config store
 * use tigertonic?