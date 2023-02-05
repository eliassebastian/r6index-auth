package config

import (
	"os"
)

func GetEnv(key, d string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		//return default value
		val = d
	}

	return val
}
