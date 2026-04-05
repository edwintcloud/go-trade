package main

import (
	"github.com/edwintcloud/go-trade/cmd"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	cmd.Execute()
}
