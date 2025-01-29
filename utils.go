package main

import (
	"fmt"
	"os"
)

func AbortOnErr(tmpl string, err error) {
	if err != nil {
		fmt.Println(fmt.Sprintf(tmpl, err.Error()))
		os.Exit(1)
	}
}

func getEnv(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
