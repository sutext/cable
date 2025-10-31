package main

import (
	"log"
	api "sutext.github.io/cable/api/kitex_gen/api/service"
)

func main() {
	svr := api.NewServer(new(ServiceImpl))

	err := svr.Run()

	if err != nil {
		log.Println(err.Error())
	}
}
