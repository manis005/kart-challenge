package store

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type Product struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Category string  `json:"category"`
}

type OrderItem struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
}

type Order struct {
	ID       string      `json:"id"`
	Items    []OrderItem `json:"items"`
	Products []Product   `json:"products"`
	Created  time.Time   `json:"created"`
}

type InMemoryStore struct {
	mu       sync.RWMutex
	products map[string]Product
	orders   map[string]Order
	nextID   int
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		products: make(map[string]Product),
		orders:   make(map[string]Order),
		nextID:   1,
	}
}

func (s *InMemoryStore) AddProduct(p Product) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.products[p.ID] = p
}

func (s *InMemoryStore) ListProducts() []Product {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Product, 0, len(s.products))
	for _, p := range s.products {
		out = append(out, p)
	}
	return out
}

func (s *InMemoryStore) GetProductByID(id string) (Product, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.products[id]
	return p, ok
}

func (s *InMemoryStore) CreateOrder(items []OrderItem) (Order, error) {
	if len(items) == 0 {
		return Order{}, errors.New("no items")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	orderID := fmt.Sprintf("%04d", s.nextID)
	s.nextID++

	products := make([]Product, 0, len(items))
	for _, it := range items {
		p, ok := s.products[it.ProductID]
		if !ok {
			return Order{}, errors.New("product not found: " + it.ProductID)
		}
		products = append(products, p)
	}

	order := Order{
		ID:       orderID,
		Items:    items,
		Products: products,
		Created:  time.Now().UTC(),
	}
	s.orders[orderID] = order
	return order, nil
}
