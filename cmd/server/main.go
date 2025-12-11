package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"
	api "github.com/manis005/kart-challenge/api"
	"github.com/manis005/kart-challenge/internal/coupons"
	"github.com/manis005/kart-challenge/internal/server"
	"github.com/manis005/kart-challenge/internal/store"
)

func main() {
	st := store.NewInMemoryStore()
	st.AddProduct(store.Product{ID: "10", Name: "Chicken Waffle", Price: 299.99, Category: "Waffle"})
	st.AddProduct(store.Product{ID: "11", Name: "Veg Burger", Price: 149.50, Category: "Burger"})

	// Use RocksDB built offline (do NOT import at startup)
	dbPath := "data/coupons.db"
	mgr, err := coupons.NewManagerFromRocks(dbPath)
	if err != nil {
		log.Fatalf("failed to open coupons db %s: %v", dbPath, err)
	}
	defer mgr.Close()

	svc := server.NewServerImpl(st, mgr)

	r := gin.Default()

	api.RegisterHandlers(r, svc)

	r.GET("/health", func(c *gin.Context) {
		c.String(200, "ok")
	})

	port := ":8080"
	log.Printf("listening on %s", port)

	if err := r.Run(port); err != nil {
		log.Fatalf("server failed: %v", err)
	}

	<-context.Background().Done()
}
