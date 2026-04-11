package types

import "time"

type User struct {
	Id        int64      `json:"id"`
	Username  string     `json:"username"`
	Name      string     `json:"name"`
	Avatar    string     `json:"avatar"`
	Access    UserAccess `json:"access"`
	Status    UserStatus `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type UserAccess string

const (
	UserAccessUser  UserAccess = "user"
	UserAccessAdmin UserAccess = "admin"
)

type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
)

type CreateUserRequest struct {
	Name     string
	Username string
	Avatar   string
	Access   UserAccess
	Status   UserStatus
}

type UpdateUserRequest struct {
	Id       int64
	Name     *string
	Avatar   *string
	Username *string
	Access   *UserAccess
	Status   *UserStatus
}

type ListUsersRequest struct {
	Page     int32
	PageSize int32
}

type ListUsersResponse struct {
	Users []*User `json:"users"`
	Total int32   `json:"total"`
}


