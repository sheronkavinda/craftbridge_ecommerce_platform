package account

type Address struct {
	ID           int64  `json:"id"`
	Label        string `json:"label"`
	FullName     string `json:"full_name"`
	Phone        string `json:"phone"`
	AddressLine1 string `json:"address_line1"`
	AddressLine2 string `json:"address_line2"`
	City         string `json:"city"`
	State        string `json:"state"`
	PostalCode   string `json:"postal_code"`
	CountryCode  string `json:"country_code"`
	IsDefault    bool   `json:"is_default"`
}

type UpdateProfileInput struct {
	Name string `json:"name"`
}

type Profile struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	Roles     []string `json:"roles"`
	CreatedAt string   `json:"created_at"`
}
