package main

import (
	"encoding/json"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go-echo-redis/db"
	"go-echo-redis/domain"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Infof(".env is not loaded properly")
	}

	db.ConnectGorm()

	e := echo.New()

	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello, I'm up! ")
	})

	e.GET("/articles", FetchArticle)
	e.GET("/articles-cache", FetchArticleWithCache)

	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "method=${method}, uri=${uri}, status=${status}\n",
	}))

	e.Logger.Fatal(e.Start(":" + os.Getenv("PORT")))
}

func FetchArticleWithCache(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limitPerPage, _ := strconv.Atoi(c.QueryParam("page_size"))
	limit, offset := getPaginateLimitOffset(page, limitPerPage)
	strLimit := strconv.Itoa(limit)
	strOffset := strconv.Itoa(offset)

	var articles []domain.Post
	//check to cache layer
	result, err := db.GetRedis().Get("article-limit-" + strLimit + "-offset-" + strOffset).Result()

	if err != nil {
		//query if key not found
		err := db.ConnectGorm().Limit(limit).Offset(offset).Find(&articles).Preload("Author").Error

		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error()).SetInternal(err)
		}

		//insert to cache layer
		data, _ := json.Marshal(articles)
		err = db.GetRedis().Set("list-catalog-offset-"+strOffset+"-limit-"+strLimit,
			data, viper.GetDuration("REDIS_CACHE_DURATION")*time.Minute).Err()

		if err != nil {
			logrus.Warn("error set cache catalog list : ", err)
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"status": "success",
			"data":   articles,
		})
	}

	err = json.Unmarshal([]byte(result), &articles)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error()).SetInternal(err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status": "success",
		"data":   articles,
	})
}

func FetchArticle(c echo.Context) error {

	page, _ := strconv.Atoi(c.QueryParam("page"))
	limitPerPage, _ := strconv.Atoi(c.QueryParam("page_size"))
	limit, offset := getPaginateLimitOffset(page, limitPerPage)

	var articles []domain.Post
	err := db.ConnectGorm().Limit(limit).Offset(offset).Preload("Author").Find(&articles).Error

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error()).SetInternal(err)
	}

	var totalArticles int64
	post := new(domain.Post)
	db.ConnectGorm().Find(post).Count(&totalArticles)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"pagination": map[string]interface{}{
			"limit":      limit,
			"pages":      page,
			"total_item": totalArticles,
		},
		"status": "success",
		"data":   articles,
	})
}

func getPaginateLimitOffset(page, limitPerPage int) (limit, offset int) {
	if page == 0 {
		page = 1
	}

	switch {
	case limitPerPage > 100:
		limitPerPage = 100
	case limitPerPage <= 0:
		limitPerPage = 10
	}

	offset = (page - 1) * limitPerPage

	return limitPerPage, offset
}
