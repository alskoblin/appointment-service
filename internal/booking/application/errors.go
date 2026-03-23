package application

import (
	"errors"

	"appointment-service/internal/apperr"
)

func IsNotFound(err error) bool {
	var appErr *apperr.AppError
	if errors.As(err, &appErr) {
		return appErr.Code == apperr.CodeNotFound
	}
	return false
}
