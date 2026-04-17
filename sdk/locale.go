package sdk

import "github.com/gin-gonic/gin"

// Locale returns the resolved locale from the request context.
// Defaults to "default" if not set.
func Locale(c *gin.Context) string {
	if l := c.GetString("locale"); l != "" {
		return l
	}
	return "base"
}
