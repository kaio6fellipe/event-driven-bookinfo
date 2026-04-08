// file: services/details/internal/adapter/inbound/http/dto.go
package http

// AddDetailRequest is the JSON body for POST /v1/details.
type AddDetailRequest struct {
	Title     string `json:"title"`
	Author    string `json:"author"`
	Year      int    `json:"year"`
	Type      string `json:"type"`
	Pages     int    `json:"pages"`
	Publisher string `json:"publisher"`
	Language  string `json:"language"`
	ISBN10    string `json:"isbn_10"`
	ISBN13    string `json:"isbn_13"`
}

// DetailResponse represents book details in API responses.
type DetailResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Year      int    `json:"year"`
	Type      string `json:"type"`
	Pages     int    `json:"pages"`
	Publisher string `json:"publisher"`
	Language  string `json:"language"`
	ISBN10    string `json:"isbn_10"`
	ISBN13    string `json:"isbn_13"`
}

// ErrorResponse is a standard error body.
type ErrorResponse struct {
	Error string `json:"error"`
}
