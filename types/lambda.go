package types

type ReqBody struct {
	Type  string `json:"type"` // post, get
	Value string `json:"value"`
}

type ResBody struct {
	Value string `json:"value"`
	Error error  `json:"error"`
}
