package api

type GetThingResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CreateThingRequest struct {
	Name string `json:"name"`
}

type ErrResp struct {
	Error string `json:"error"`
}
