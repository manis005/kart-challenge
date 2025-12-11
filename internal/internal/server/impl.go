package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	api "github.com/manis005/kart-challenge/api"
	"github.com/manis005/kart-challenge/internal/coupons"
	"github.com/manis005/kart-challenge/internal/store"
)

// ServerImpl implements api.ServerInterface (Gin-based).
type ServerImpl struct {
	Store   *store.InMemoryStore
	Coupons *coupons.Manager
	APIKey  string
}

// NewServerImpl constructs the service implementation.
func NewServerImpl(st *store.InMemoryStore, mgr *coupons.Manager) *ServerImpl {
	return &ServerImpl{
		Store:   st,
		Coupons: mgr,
		APIKey:  "apitest",
	}
}

// helper to return error responses in same shape as ApiResponse (code,type,message)
func writeAPIError(c *gin.Context, httpStatus int, code int, typ string, message string) {
	c.JSON(httpStatus, map[string]interface{}{
		"code":    code,
		"type":    typ,
		"message": message,
	})
}

// ListProducts handles GET /product
// Signature matches api.ServerInterface: ListProducts(c *gin.Context)
func (s *ServerImpl) ListProducts(c *gin.Context) {
	prods := s.Store.ListProducts()

	// convert []store.Product -> []api.Product (with pointer fields)
	out := make([]api.Product, 0, len(prods))
	for _, p := range prods {
		id := p.ID
		name := p.Name
		category := p.Category
		price := float32(p.Price)
		out = append(out, api.Product{
			Category: &category,
			Id:       &id,
			Name:     &name,
			Price:    &price,
		})
	}

	c.JSON(http.StatusOK, out)
}

// GetProduct handles GET /product/{productId}
// Signature matches api.ServerInterface: GetProduct(c *gin.Context, productId int64)
func (s *ServerImpl) GetProduct(c *gin.Context, productId int64) {
	// convert int64 to string because store keys are strings like "10"
	idStr := strconv.FormatInt(productId, 10)
	p, ok := s.Store.GetProductByID(idStr)
	if !ok {
		// spec expects 404 when not found
		c.String(http.StatusNotFound, "product not found")
		return
	}

	// convert to api.Product
	id := p.ID
	name := p.Name
	category := p.Category
	price := float32(p.Price)

	c.JSON(http.StatusOK, api.Product{
		Category: &category,
		Id:       &id,
		Name:     &name,
		Price:    &price,
	})
}

// PlaceOrder handles POST /order
// Signature matches api.ServerInterface: PlaceOrder(c *gin.Context)
func (s *ServerImpl) PlaceOrder(c *gin.Context) {
	// check api_key header (header name is "api_key" per spec)
	ak := c.GetHeader("api_key")
	if ak == "" || ak != s.APIKey {
		writeAPIError(c, http.StatusBadRequest, 400, "error", "invalid or missing api_key")
		return
	}

	var req api.OrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, 400, "error", "invalid JSON body")
		return
	}

	// items required
	if len(req.Items) == 0 {
		writeAPIError(c, http.StatusUnprocessableEntity, 422, "validation_error", "items is required and must be non-empty")
		return
	}

	// validate each item and convert to store.OrderItem
	items := make([]store.OrderItem, 0, len(req.Items))
	for _, it := range req.Items {
		pid := strings.TrimSpace(it.ProductId)
		qty := int(it.Quantity)
		if pid == "" || qty <= 0 {
			writeAPIError(c, http.StatusUnprocessableEntity, 422, "validation_error", "each item must have productId and quantity > 0")
			return
		}
		items = append(items, store.OrderItem{ProductID: pid, Quantity: qty})
	}

	// coupon validation if provided
	if req.CouponCode != nil && strings.TrimSpace(*req.CouponCode) != "" {
		if !s.Coupons.IsValidPromo(*req.CouponCode) {
			writeAPIError(c, http.StatusUnprocessableEntity, 422, "validation_error", "invalid coupon code")
			return
		}
	}

	// create order in store
	order, err := s.Store.CreateOrder(items)
	if err != nil {
		writeAPIError(c, http.StatusBadRequest, 400, "error", "failed to create order: "+err.Error())
		return
	}

	// Build api.Order response using generated types (note Order struct uses pointers)
	// Build Items: *[]struct{ ProductId *string; Quantity *int }
	apiItems := make([]struct {
		ProductId *string `json:"productId,omitempty"`
		Quantity  *int    `json:"quantity,omitempty"`
	}, 0, len(order.Items))
	for _, it := range order.Items {
		pid := it.ProductID
		qty := it.Quantity
		apiItems = append(apiItems, struct {
			ProductId *string `json:"productId,omitempty"`
			Quantity  *int    `json:"quantity,omitempty"`
		}{
			ProductId: &pid,
			Quantity:  &qty,
		})
	}

	// Build Products slice: []api.Product
	apiProducts := make([]api.Product, 0, len(order.Products))
	for _, p := range order.Products {
		id := p.ID
		name := p.Name
		category := p.Category
		price := float32(p.Price)
		apiProducts = append(apiProducts, api.Product{
			Category: &category,
			Id:       &id,
			Name:     &name,
			Price:    &price,
		})
	}

	// prepare api.Order (using pointers)
	id := order.ID
	apiOrder := api.Order{
		Id:       &id,
		Items:    &apiItems,
		Products: &apiProducts,
	}

	c.JSON(http.StatusOK, apiOrder)
}
