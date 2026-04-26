package api

import "time"

type GetThingResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"created_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

type CreateThingRequest struct {
	Name string `json:"name"`
}

type ErrResp struct {
	Error string `json:"error"`
}
