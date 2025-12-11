package main

import (
	"context"
	"log"
	"os"

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

	couponFiles := []string{
		"data/couponbase1.gz",
		"data/couponbase2.gz",
		"data/couponbase3.gz",
	}

	for _, p := range couponFiles {
		if _, err := os.Stat(p); err != nil {
			log.Fatalf("coupon file missing: %s (put all three files under data/)", p)
		}
	}

	mgr, err := coupons.NewManagerFromFiles(couponFiles)
	if err != nil {
		log.Fatalf("failed to load coupons: %v", err)
	}
	defer mgr.Close()

	log.Printf("coupon manager loaded (%d codes)", len(mgr.Snapshot()))

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
