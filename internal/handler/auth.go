package handler

import (
	"net/http"

	userservice "GPTBridge/internal/domain/user/service"
	"github.com/gin-gonic/gin"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleLogin 处理用户登录。
func (r *Router) handleLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}

	user, token, err := r.auth.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		status := http.StatusUnauthorized
		if err == userservice.ErrUserDisabled {
			status = http.StatusForbidden
		}
		writeError(c, status, err)
		return
	}

	c.SetCookie(r.auth.CookieName(), token, r.auth.CookieMaxAge(), "/", "", r.auth.CookieSecure(), true)
	c.JSON(http.StatusOK, gin.H{"user": user})
}

// handleLogout 处理用户退出。
func (r *Router) handleLogout(c *gin.Context) {
	token, _ := c.Cookie(r.auth.CookieName())
	if err := r.auth.Logout(c.Request.Context(), token); err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	c.SetCookie(r.auth.CookieName(), "", -1, "/", "", r.auth.CookieSecure(), true)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// handleMe 返回当前登录用户。
func (r *Router) handleMe(c *gin.Context) {
	user, ok := userservice.CurrentUserFromContext(c.Request.Context())
	if !ok {
		writeError(c, http.StatusUnauthorized, userservice.ErrInvalidSession)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}
