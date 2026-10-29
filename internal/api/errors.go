package api

import (
	"errors"
	"log"
	"net/http"

	"github.com/gofiber/fiber/v2"
)

type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

func newErr(status int, code, msg string) *APIError {
	return &APIError{Status: status, Code: code, Message: msg}
}

func badRequest(msg string) *APIError {
	return newErr(http.StatusBadRequest, "INVALID_REQUEST", msg)
}

func unauthorized(msg string) *APIError {
	return newErr(http.StatusUnauthorized, "UNAUTHORIZED", msg)
}

func errorBody(code, msg string) fiber.Map {
	return fiber.Map{"error": fiber.Map{"code": code, "message": msg}}
}

func errorHandler(c *fiber.Ctx, err error) error {
	var ae *APIError
	if errors.As(err, &ae) {
		return c.Status(ae.Status).JSON(errorBody(ae.Code, ae.Message))
	}
	var fe *fiber.Error
	if errors.As(err, &fe) {
		code := "ERROR"
		switch fe.Code {
		case http.StatusNotFound:
			code = "NOT_FOUND"
		case http.StatusMethodNotAllowed:
			code = "METHOD_NOT_ALLOWED"
		case http.StatusRequestEntityTooLarge:
			code = "PAYLOAD_TOO_LARGE"
		case http.StatusTooManyRequests:
			code = "RATE_LIMITED"
		}
		return c.Status(fe.Code).JSON(errorBody(code, fe.Message))
	}
	log.Printf("internal error: %s %s: %v", c.Method(), c.Path(), err)
	return c.Status(http.StatusInternalServerError).JSON(errorBody("INTERNAL", "something went wrong"))
}
