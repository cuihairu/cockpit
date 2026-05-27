package storage

import (
	"testing"
	"time"
)

func TestUserTOTPFields(t *testing.T) {
	setupTime := time.Now()
	user := &User{
		Username:    "testuser",
		Password:    "hashedpassword",
		TOTPSecret:  "encrypted_secret",
		TOTPEnabled: true,
		BackupCodes: `["hash1","hash2"]`,
		TOTPSetupAt: &setupTime,
	}

	if user.TOTPSecret != "encrypted_secret" {
		t.Errorf("TOTPSecret mismatch: got %s, want %s", user.TOTPSecret, "encrypted_secret")
	}
	if !user.TOTPEnabled {
		t.Error("TOTPEnabled should be true")
	}
	if user.BackupCodes != `["hash1","hash2"]` {
		t.Errorf("BackupCodes mismatch: got %s, want %s", user.BackupCodes, `["hash1","hash2"]`)
	}
	if user.TOTPSetupAt == nil {
		t.Error("TOTPSetupAt should not be nil")
	}
}

func TestCreateUser(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	user := &User{
		Username: "testuser",
		Password: "hashedpass",
		Email:    "test@example.com",
		Role:     "user",
	}
	if err := db.CreateUser(user); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if user.ID == "" {
		t.Error("User.ID should be set by BeforeCreate hook")
	}
}

func TestGetUserByUsername(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.CreateUser(&User{Username: "alice", Password: "hash", Role: "admin"})

	user, err := db.GetUserByUsername("alice")
	if err != nil {
		t.Fatalf("GetUserByUsername() error = %v", err)
	}
	if user.Username != "alice" {
		t.Errorf("Username = %v, want alice", user.Username)
	}
	if user.Role != "admin" {
		t.Errorf("Role = %v, want admin", user.Role)
	}
}

func TestGetUserByUsernameNotFound(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	_, err := db.GetUserByUsername("nobody")
	if err != ErrNotFound {
		t.Errorf("GetUserByUsername(nobody) error = %v, want ErrNotFound", err)
	}
}

func TestGetUserByID(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	user := &User{Username: "bob", Password: "hash", Role: "user"}
	db.CreateUser(user)

	got, err := db.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	if got.Username != "bob" {
		t.Errorf("Username = %v, want bob", got.Username)
	}
}

func TestGetUserByIDNotFound(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	_, err := db.GetUserByID("nonexistent")
	if err != ErrNotFound {
		t.Errorf("GetUserByID(nonexistent) error = %v, want ErrNotFound", err)
	}
}

func TestListUsers(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.CreateUser(&User{Username: "alice", Role: "admin"})
	db.CreateUser(&User{Username: "bob", Role: "user"})

	users, err := db.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if len(users) != 2 {
		t.Errorf("ListUsers() count = %d, want 2", len(users))
	}
	// Passwords should not be returned (selected columns exclude password)
	for _, u := range users {
		if u.Password != "" {
			t.Errorf("User %s password should be empty in list", u.Username)
		}
	}
}

func TestUpdateUser(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	user := &User{Username: "alice", Email: "old@example.com", Role: "user"}
	db.CreateUser(user)

	user.Email = "new@example.com"
	user.Role = "admin"
	if err := db.UpdateUser(user); err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}

	got, _ := db.GetUserByID(user.ID)
	if got.Email != "new@example.com" {
		t.Errorf("Email = %v, want new@example.com", got.Email)
	}
	if got.Role != "admin" {
		t.Errorf("Role = %v, want admin", got.Role)
	}
}

func TestUpdatePassword(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	user := &User{Username: "alice", Password: "oldhash"}
	db.CreateUser(user)

	if err := db.UpdatePassword(user.ID, "newhash"); err != nil {
		t.Fatalf("UpdatePassword() error = %v", err)
	}

	got, _ := db.GetUserByUsername("alice")
	if got.Password != "newhash" {
		t.Errorf("Password = %v, want newhash", got.Password)
	}
}

func TestDeleteUser(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	user := &User{Username: "alice", Password: "hash"}
	db.CreateUser(user)

	if err := db.DeleteUser(user.ID); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}

	_, err := db.GetUserByUsername("alice")
	if err != ErrNotFound {
		t.Errorf("GetUserByUsername() after delete error = %v, want ErrNotFound", err)
	}
}

func TestVerifyPasswordIntegration(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	hashed, err := HashPassword("secret123")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	db.CreateUser(&User{Username: "alice", Password: hashed, Role: "admin"})

	user, err := db.VerifyPassword("alice", "secret123")
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if user.Username != "alice" {
		t.Errorf("Username = %v, want alice", user.Username)
	}
	if user.Password != "" {
		t.Error("Password should be cleared after verification")
	}
}

func TestVerifyPasswordWrongPass(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	hashed, _ := HashPassword("secret123")
	db.CreateUser(&User{Username: "alice", Password: hashed})

	_, err := db.VerifyPassword("alice", "wrongpass")
	if err != ErrNotFound {
		t.Errorf("VerifyPassword(wrong) error = %v, want ErrNotFound", err)
	}
}

func TestVerifyPasswordNoUser(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	_, err := db.VerifyPassword("nobody", "whatever")
	if err != ErrNotFound {
		t.Errorf("VerifyPassword(nobody) error = %v, want ErrNotFound", err)
	}
}

func TestInitAdminUser(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	if err := db.InitAdminUser("admin", "admin123"); err != nil {
		t.Fatalf("InitAdminUser() error = %v", err)
	}

	user, err := db.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("GetUserByUsername() error = %v", err)
	}
	if user.Role != "admin" {
		t.Errorf("Role = %v, want admin", user.Role)
	}
}

func TestInitAdminUserAlreadyExists(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	db.InitAdminUser("admin", "pass1")
	// Second call should not fail
	if err := db.InitAdminUser("admin", "pass2"); err != nil {
		t.Fatalf("InitAdminUser() duplicate should not error: %v", err)
	}

	user, _ := db.GetUserByUsername("admin")
	// Password should remain unchanged
	if !verifyPassword(user.Password, "pass1") {
		t.Error("Password should be pass1 (unchanged)")
	}
}

func TestHashPasswordAndVerify(t *testing.T) {
	hashed, err := HashPassword("mypassword")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if !verifyPassword(hashed, "mypassword") {
		t.Error("verifyPassword should return true for correct password")
	}
	if verifyPassword(hashed, "wrongpassword") {
		t.Error("verifyPassword should return false for wrong password")
	}
}
