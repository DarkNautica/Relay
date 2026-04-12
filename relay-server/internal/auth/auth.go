package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Validate checks a Pusher-compatible auth signature for private/presence channels.
//
// The auth token format is: "app-key:hmac-sha256-signature"
// The signature is HMAC-SHA256 of "{socketId}:{channelName}" using the app secret.
// For presence channels, the signature covers "{socketId}:{channelName}:{channelData}".
func Validate(appKey, appSecret, socketID, channelName, authToken, channelData string) bool {
	if authToken == "" {
		return false
	}

	// Split token into key:signature
	parts := strings.SplitN(authToken, ":", 2)
	if len(parts) != 2 {
		return false
	}

	tokenKey := parts[0]
	tokenSig := parts[1]

	// Key must match our app key
	if tokenKey != appKey {
		return false
	}

	// Build the string to sign
	stringToSign := fmt.Sprintf("%s:%s", socketID, channelName)
	if channelData != "" {
		stringToSign = fmt.Sprintf("%s:%s:%s", socketID, channelName, channelData)
	}

	// Compute expected signature
	expected := computeHMAC(appSecret, stringToSign)

	// Constant-time comparison to prevent timing attacks
	return hmac.Equal([]byte(tokenSig), []byte(expected))
}

// Sign creates an auth token for a given socket ID and channel name.
// This is used by the PHP driver and server-side auth endpoint.
func Sign(appKey, appSecret, socketID, channelName, channelData string) string {
	stringToSign := fmt.Sprintf("%s:%s", socketID, channelName)
	if channelData != "" {
		stringToSign = fmt.Sprintf("%s:%s:%s", socketID, channelName, channelData)
	}

	sig := computeHMAC(appSecret, stringToSign)
	return fmt.Sprintf("%s:%s", appKey, sig)
}

// computeHMAC returns the HMAC-SHA256 hex digest of a message.
func computeHMAC(secret, message string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}
