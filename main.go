package main

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func main() {
	p := getConsulProvider("localhost:8080", "snowflake/worker/id/")
	initSnowflake(p)
	router := gin.Default()
	router.GET("/id", nextId)
	router.Run("127.0.0.1:8080")
}

func nextId(c *gin.Context) {
	id := snowflake.NextId()
	c.IndentedJSON(http.StatusOK, id)
}
