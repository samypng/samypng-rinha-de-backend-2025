package types

type Payment struct {
	CorrelationID      string  `json:"correlationId" validate:"required,uuid"`
	Amount             float64 `json:"amount" validate:"required,gt=0"`
	RequestedAt        string  `json:"requestedAt"`
	IsDefaultProcessor bool    `json:"isDefaultProcessor"`
}

type PaymentHostHealthStatusPayload struct {
	Failing         bool `json:"failing"`
	MinResponseTime int  `json:"minResponseTime"`
}

type PaymentSummary struct {
	TotalRequests int64   `json:"totalRequests"`
	TotalAmount   float64 `json:"totalAmount"`
}
