package server

import (
	"awesomeProjectFaraway/internal/pkg/cache"
	"awesomeProjectFaraway/internal/pkg/clock"
	"awesomeProjectFaraway/internal/pkg/config"
	"awesomeProjectFaraway/internal/server"
	"context"
	"fmt"
	"math/rand"
	"time"
)

func main() {
	fmt.Println("start server")

	// loading config from file and env
	configInst, err := config.Load("config/config.json")
	if err != nil {
		fmt.Println("error load config:", err)
		return
	}

	// init context to pass config down
	ctx := context.Background()
	ctx = context.WithValue(ctx, "config", configInst)
	sysClock := clock.SystemClock{}
	ctx = context.WithValue(ctx, "clock", sysClock)

	cacheInst := cache.InitInMemoryCache(sysClock)
	ctx = context.WithValue(ctx, "cache", cacheInst)

	// seed random generator to randomize order of quotes
	rand.Seed(time.Now().UnixNano())

	// run server
	serverAddress := fmt.Sprintf("%s:%d", configInst.ServerHost, configInst.ServerPort)
	err = server.Run(ctx, serverAddress)
	if err != nil {
		fmt.Println("server error:", err)
	}
}
