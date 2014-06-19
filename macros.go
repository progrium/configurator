package main

func loadBuiltinMacros(preprocessor *Preprocessor, store ConfigStore, config *Config) {
	preprocessor.Register("$value", func(input macroinput) interface{} {
		path := input["$value"].(string)
		go store.WatchToUpdate(config, path)
		value := store.Get(path)
		if value != "" {
			return value
		}
		if input["default"] != nil {
			return input["default"].(string)
		}
		return ""
	})

	preprocessor.Register("$file", func(input macroinput) interface{} {
		path := input["$file"].(string)
		go store.WatchToUpdate(config, path)
		value := store.Get(path)
		if value != "" {
			return value
		}
		if input["default"] != nil {
			return input["default"].(string)
		}
		return ""
	})

	// $keys
	// $file
	// $environ

	// $for
	// $if
	// $eq
	// $ne

	// $service
	// $services
}
