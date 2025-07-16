package types

type Payment struct {
	CorrelationID string  `json:"correlationId"`
	Amount        float64 `json:"amount"`
	RequestedAt   string  `json:"requestedAt"`
}

type PaymentHostHealthStatusPayload struct {
	Failing         bool `json:"failing"`
	MinResponseTime int  `json:"min_response_time"`
}
