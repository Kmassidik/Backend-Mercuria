# 🔐 Mercuria Auth Service API Testing Guide

## 📋 Overview

Base URL: `http://localhost:8080`

All endpoints accept and return `application/json`

---

## 🚀 Quick Test Sequence

Run these commands in order to test the complete auth flow:

```bash
# 1. Health Check
curl -X GET http://localhost:8080/health

# 2. Register a new user
curl -X POST http://localhost:8080/api/v1/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "SecurePassword123!",
    "full_name": "Test User"
  }'

# 3. Login (save the tokens from response)
curl -X POST http://localhost:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "SecurePassword123!"
  }'

# 4. Get current user info (replace YOUR_ACCESS_TOKEN)
curl -X GET http://localhost:8080/api/v1/me \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"

# 5. Refresh token (replace YOUR_REFRESH_TOKEN)
curl -X POST http://localhost:8080/api/v1/refresh \
  -H "Content-Type: application/json" \
  -d '{
    "refresh_token": "YOUR_REFRESH_TOKEN"
  }'
```

---

## 📚 Detailed API Documentation

### 1. Health Check

**Endpoint:** `GET /health`

**Description:** Check if the service is running

**Request:**

```bash
curl -X GET http://localhost:8080/health
```

**Expected Response (200 OK):**

```json
{
  "status": "healthy"
}
```

---

### 2. Register New User

**Endpoint:** `POST /api/v1/register`

**Description:** Create a new user account

**Request:**

```bash
curl -X POST http://localhost:8080/api/v1/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "john.doe@example.com",
    "password": "MySecurePass123!",
    "full_name": "John Doe"
  }'
```

**Request Body:**

```json
{
  "email": "john.doe@example.com",
  "password": "MySecurePass123!",
  "full_name": "John Doe"
}
```

**Expected Response (201 Created):**

```json
{
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "john.doe@example.com",
    "full_name": "John Doe",
    "created_at": "2025-11-11T10:30:00Z"
  },
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_in": 900
}
```

**Error Response (400 Bad Request):**

```json
{
  "error": "email already exists"
}
```

**Validation Rules:**

- Email must be valid format
- Password minimum 8 characters
- Full name is required

---

### 3. Login

**Endpoint:** `POST /api/v1/login`

**Description:** Authenticate user and receive JWT tokens

**Request:**

```bash
curl -X POST http://localhost:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "john.doe@example.com",
    "password": "MySecurePass123!"
  }'
```

**Request Body:**

```json
{
  "email": "john.doe@example.com",
  "password": "MySecurePass123!"
}
```

**Expected Response (200 OK):**

```json
{
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "john.doe@example.com",
    "full_name": "John Doe",
    "created_at": "2025-11-11T10:30:00Z"
  },
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoiNTUwZTg0MDAtZTI5Yi00MWQ0LWE3MTYtNDQ2NjU1NDQwMDAwIiwiZW1haWwiOiJqb2huLmRvZUBleGFtcGxlLmNvbSIsImV4cCI6MTY5OTcwNDYwMH0.xyz...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiI1NTBlODQwMC1lMjliLTQxZDQtYTcxNi00NDY2NTU0NDAwMDAiLCJleHAiOjE3MDA1Njg2MDB9.abc...",
  "expires_in": 900
}
```

**Error Response (401 Unauthorized):**

```json
{
  "error": "invalid credentials"
}
```

---

### 4. Get Current User

**Endpoint:** `GET /api/v1/me`

**Description:** Get authenticated user's information

**Authentication:** Required (Bearer token)

**Request:**

```bash
curl -X GET http://localhost:8080/api/v1/me \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

**Expected Response (200 OK):**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "john.doe@example.com",
  "full_name": "John Doe",
  "created_at": "2025-11-11T10:30:00Z"
}
```

**Error Response (401 Unauthorized):**

```json
{
  "error": "missing authorization header"
}
```

```json
{
  "error": "invalid or expired token"
}
```

---

### 5. Refresh Access Token

**Endpoint:** `POST /api/v1/refresh`

**Description:** Get a new access token using refresh token

**Request:**

```bash
curl -X POST http://localhost:8080/api/v1/refresh \
  -H "Content-Type: application/json" \
  -d '{
    "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
  }'
```

**Request Body:**

```json
{
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

**Expected Response (200 OK):**

```json
{
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "john.doe@example.com",
    "full_name": "John Doe",
    "created_at": "2025-11-11T10:30:00Z"
  },
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_in": 900
}
```

**Error Response (401 Unauthorized):**

```json
{
  "error": "invalid or expired refresh token"
}
```

**Note:** The refresh token is rotated (a new one is issued and the old one is invalidated)

---

## 🧪 Automated Test Script

Save this as `test_auth.sh`:

```bash
#!/bin/bash

BASE_URL="http://localhost:8080"
EMAIL="test_$(date +%s)@example.com"
PASSWORD="SecurePass123!"
FULL_NAME="Test User"

echo "🔍 Testing Mercuria Auth Service"
echo "================================="

# Test 1: Health Check
echo ""
echo "1️⃣ Testing Health Check..."
HEALTH=$(curl -s -X GET "$BASE_URL/health")
echo "Response: $HEALTH"

if echo "$HEALTH" | grep -q "healthy"; then
    echo "✅ Health check passed"
else
    echo "❌ Health check failed"
    exit 1
fi

# Test 2: Register
echo ""
echo "2️⃣ Testing Registration..."
REGISTER_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/register" \
  -H "Content-Type: application/json" \
  -d "{
    \"email\": \"$EMAIL\",
    \"password\": \"$PASSWORD\",
    \"full_name\": \"$FULL_NAME\"
  }")

echo "Response: $REGISTER_RESPONSE"

if echo "$REGISTER_RESPONSE" | grep -q "access_token"; then
    echo "✅ Registration passed"
    ACCESS_TOKEN=$(echo "$REGISTER_RESPONSE" | grep -o '"access_token":"[^"]*' | cut -d'"' -f4)
    REFRESH_TOKEN=$(echo "$REGISTER_RESPONSE" | grep -o '"refresh_token":"[^"]*' | cut -d'"' -f4)
else
    echo "❌ Registration failed"
    exit 1
fi

# Test 3: Login
echo ""
echo "3️⃣ Testing Login..."
LOGIN_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/login" \
  -H "Content-Type: application/json" \
  -d "{
    \"email\": \"$EMAIL\",
    \"password\": \"$PASSWORD\"
  }")

echo "Response: $LOGIN_RESPONSE"

if echo "$LOGIN_RESPONSE" | grep -q "access_token"; then
    echo "✅ Login passed"
    ACCESS_TOKEN=$(echo "$LOGIN_RESPONSE" | grep -o '"access_token":"[^"]*' | cut -d'"' -f4)
else
    echo "❌ Login failed"
    exit 1
fi

# Test 4: Get Current User
echo ""
echo "4️⃣ Testing Get Current User..."
ME_RESPONSE=$(curl -s -X GET "$BASE_URL/api/v1/me" \
  -H "Authorization: Bearer $ACCESS_TOKEN")

echo "Response: $ME_RESPONSE"

if echo "$ME_RESPONSE" | grep -q "$EMAIL"; then
    echo "✅ Get current user passed"
else
    echo "❌ Get current user failed"
    exit 1
fi

# Test 5: Refresh Token
echo ""
echo "5️⃣ Testing Token Refresh..."
REFRESH_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/refresh" \
  -H "Content-Type: application/json" \
  -d "{
    \"refresh_token\": \"$REFRESH_TOKEN\"
  }")

echo "Response: $REFRESH_RESPONSE"

if echo "$REFRESH_RESPONSE" | grep -q "access_token"; then
    echo "✅ Token refresh passed"
else
    echo "❌ Token refresh failed"
    exit 1
fi

# Test 6: Invalid Login
echo ""
echo "6️⃣ Testing Invalid Login..."
INVALID_LOGIN=$(curl -s -X POST "$BASE_URL/api/v1/login" \
  -H "Content-Type: application/json" \
  -d "{
    \"email\": \"$EMAIL\",
    \"password\": \"WrongPassword\"
  }")

echo "Response: $INVALID_LOGIN"

if echo "$INVALID_LOGIN" | grep -q "error"; then
    echo "✅ Invalid login properly rejected"
else
    echo "❌ Invalid login should have failed"
fi

echo ""
echo "================================="
echo "🎉 All tests completed!"
```

**Run the test:**

```bash
chmod +x test_auth.sh
./test_auth.sh
```

---

## 🛠️ Troubleshooting

### Service won't start?

**Check if port 8080 is already in use:**

```bash
lsof -i :8080
# or
netstat -tulnp | grep 8080
```

**Check database connection:**

```bash
docker exec -it mercuria-postgres-test psql -U postgres -d mercuria_auth -c "SELECT 1;"
```

**Check Redis connection:**

```bash
docker exec -it mercuria-redis-test redis-cli ping
```

### Database not created?

```bash
docker exec -it mercuria-postgres-test psql -U postgres -c "CREATE DATABASE mercuria_auth;"
```

### Migrations not applied?

```bash
# You'll need to apply your migration files
docker exec -i mercuria-postgres-test psql -U postgres -d mercuria_auth < migrations/auth/001_create_users_table.sql
```

---

## 📊 HTTP Status Codes

| Status Code               | Meaning            | When                               |
| ------------------------- | ------------------ | ---------------------------------- |
| 200 OK                    | Success            | Login, Refresh, Get User           |
| 201 Created               | Resource created   | Registration                       |
| 400 Bad Request           | Invalid input      | Validation errors                  |
| 401 Unauthorized          | Auth failed        | Invalid credentials, expired token |
| 404 Not Found             | Resource not found | User doesn't exist                 |
| 500 Internal Server Error | Server error       | Database or system errors          |

---

## 🔐 Security Notes

1. **Access Token:** Short-lived (15 minutes default), use for API requests
2. **Refresh Token:** Long-lived (7 days default), use to get new access tokens
3. **Token Rotation:** Refresh tokens are rotated on each use (old one invalidated)
4. **Password Requirements:** Minimum 8 characters (configure in validation)
5. **HTTPS:** Use HTTPS in production, never HTTP

---

## 🎯 Next Steps

1. ✅ Test all endpoints using the cURL commands above
2. ✅ Run the automated test script
3. ✅ Check logs for any errors
4. ✅ Verify JWT tokens using [jwt.io](https://jwt.io)
5. ✅ Move on to Wallet Service implementation

---

**Happy Testing! 🚀**
