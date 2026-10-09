package user

type CreateUserRequest struct {
	FirstName string `json:"first_name" binding:"required,min=2,max=80"`
	LastName  string `json:"last_name"  binding:"required,min=2,max=80"`
	Email     string `json:"email"      binding:"required,email,max=160"`
	Phone     string `json:"phone"      binding:"omitempty,max=20"`
	Password  string `json:"password"   binding:"required,min=8,max=72"`
}

type UpdateUserRequest struct {
	FirstName *string `json:"first_name" binding:"omitempty,min=2,max=80"`
	LastName  *string `json:"last_name"  binding:"omitempty,min=2,max=80"`
	Email     *string `json:"email"      binding:"omitempty,email,max=160"`
	Phone     *string `json:"phone"      binding:"omitempty,max=20"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	Password        string `json:"password"         binding:"required,min=8,max=72"`
}

type DeleteAccountRequest struct {
	Password string `json:"password" binding:"required"`
}

type UserResponse struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Phone     string `json:"phone,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
