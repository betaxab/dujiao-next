package public

import handlershared "github.com/dujiao-next/internal/http/handlers/shared"

func normalizePagination(page, pageSize int) (int, int) {
	return handlershared.NormalizePagination(page, pageSize)
}
