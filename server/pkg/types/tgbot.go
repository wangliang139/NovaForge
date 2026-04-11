package types

type SendCodeRequest struct {
	AppId       string `json:"app_id"`
	AppHash     string `json:"app_hash"`
	PhoneNumber string `json:"phone_number"`
}

type GetSessionRequest struct {
	AppId       string `json:"app_id"`
	AppHash     string `json:"app_hash"`
	PhoneNumber string `json:"phone_number"`
	CodeHash    string `json:"code_hash"`
	Code        string `json:"code"`
	Session     string `json:"session"`
}

type GetSessionResponse struct {
	Success bool   `json:"success"`
	Session string `json:"session"`
	Message string `json:"message"`
}

type SendCodeResponse struct {
	Success  bool   `json:"success"`
	CodeHash string `json:"code_hash"`
	Message  string `json:"message"`
	Session  string `json:"session"`
}
