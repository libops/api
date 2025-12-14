package service

import (
	"database/sql"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/go-sql-driver/mysql"
)

// Common error messages that don't disclose resource existence.
const (
	ErrMsgNotFound           = "resource not found"
	ErrMsgInvalidInput       = "invalid input"
	ErrMsgUnauthorized       = "unauthorized"
	ErrMsgAlreadyExists      = "resource already exists"
	ErrMsgPreconditionFailed = "operation cannot be completed"
	ErrMsgInternalError      = "internal server error"
)

// HandleDatabaseError converts database errors to appropriate ConnectRPC errors
// without disclosing sensitive information about resource existence.
func HandleDatabaseError(err error, resourceType string) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		// Don't disclose which specific resource wasn't found
		return connect.NewError(connect.CodeNotFound, errors.New(ErrMsgNotFound))
	}

	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case 1062:
			return connect.NewError(connect.CodeAlreadyExists, errors.New(ErrMsgAlreadyExists))
		case 1451:
			return connect.NewError(connect.CodeFailedPrecondition, errors.New(ErrMsgPreconditionFailed))
		case 1452:
			return connect.NewError(connect.CodeInvalidArgument, errors.New(ErrMsgInvalidInput))
		}
	}

	// Generic internal error - don't expose implementation details
	return connect.NewError(connect.CodeInternal, errors.New(ErrMsgInternalError))
}

// NotFoundError returns a generic "not found" error without resource details.
func NotFoundError() error {
	return connect.NewError(connect.CodeNotFound, errors.New(ErrMsgNotFound))
}

// InvalidInputError returns a generic "invalid input" error with field context.
func InvalidInputError(field string) error {
	return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s: %s", field, ErrMsgInvalidInput))
}

// AlreadyExistsError returns a generic "already exists" error.
func AlreadyExistsError() error {
	return connect.NewError(connect.CodeAlreadyExists, errors.New(ErrMsgAlreadyExists))
}

// PreconditionFailedError returns a generic precondition failure error.
func PreconditionFailedError() error {
	return connect.NewError(connect.CodeFailedPrecondition, errors.New(ErrMsgPreconditionFailed))
}

// InternalError returns a generic internal error.
func InternalError() error {
	return connect.NewError(connect.CodeInternal, errors.New(ErrMsgInternalError))
}

// UnauthorizedError returns a generic unauthorized error.
func UnauthorizedError() error {
	return connect.NewError(connect.CodeUnauthenticated, errors.New(ErrMsgUnauthorized))
}

// PermissionDeniedError returns a generic permission denied error.
func PermissionDeniedError() error {
	return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("permission denied"))
}
