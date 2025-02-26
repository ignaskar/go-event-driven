package entities

type Ticket struct {
	TicketID      string `json:"ticket_id"`
	Price         Money  `json:"price"`
	CustomerEmail string `json:"customer_email"`
}
