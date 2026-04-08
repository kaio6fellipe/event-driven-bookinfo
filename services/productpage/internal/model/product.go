// file: services/productpage/internal/model/product.go
package model

// ProductDetail is the aggregated view model for a product detail page.
type ProductDetail struct {
	ID        string
	Title     string
	Author    string
	Year      int
	Type      string
	Pages     int
	Publisher string
	Language  string
	ISBN10    string
	ISBN13    string
}

// ProductReview is the view model for a single review.
type ProductReview struct {
	ID       string
	Reviewer string
	Text     string
	Average  float64
	Count    int
}

// ProductPage is the full page view model combining detail and reviews.
type ProductPage struct {
	Detail  *ProductDetail
	Reviews []ProductReview
}

// Product is a summary view model for listing.
type Product struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
