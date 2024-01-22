package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	t "github.com/moreSalt/url-shortener/types"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/btcsuite/btcutil/base58"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {

	lamb := os.Getenv("LAMBDA")

	if lamb == "true" {
		lambda.Start(lambdaMain)
	} else {
		err := godotenv.Load(".env")
		if err != nil {
			log.Fatal("Error loading .env file")
		}

		// GOOGLEAPIKEY := os.Getenv("GOOGLE_API_KEY")
		PGHOST := os.Getenv("PG_HOST")
		PGPORT := os.Getenv("PG_PORT")
		PGUSER := os.Getenv("PG_USER")
		PGDB := os.Getenv("PG_DB")
		PGPASSWORD := os.Getenv("PG_PASSWORD")

		db, err := connect(PGUSER, PGPASSWORD, PGHOST, PGDB, PGPORT)
		if err != nil {
			log.Fatal(err)
		}

		defer db.Close()

		err = db.Ping()
		if err != nil {
			log.Fatal(err)
		}

		link, err := shortenUrl("https://google.com", db)
		if err != nil {
			log.Fatal(err)
		}
		log.Println(link)
	}

}

func lambdaMain(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	headers := map[string]string{
		"Access-Control-Allow-Headers": "Content-Type",
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "OPTIONS,POST,GET",
	}

	var reqBody t.ReqBody
	err := json.Unmarshal([]byte(req.Body), &reqBody)
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode:      400,
			Body:            "Failed to read req body",
			IsBase64Encoded: false,
		}, err
	}

	// GOOGLEAPIKEY := os.Getenv("GOOGLE_API_KEY")
	PGHOST := os.Getenv("PG_HOST")
	PGPORT := os.Getenv("PG_PORT")
	PGUSER := os.Getenv("PG_USER")
	PGDB := os.Getenv("PG_DB")
	PGPASSWORD := os.Getenv("PG_PASSWORD")

	db, err := connect(PGUSER, PGPASSWORD, PGHOST, PGDB, PGPORT)
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode:      400,
			Body:            string("Failed to connect to db"),
			Headers:         headers,
			IsBase64Encoded: false,
		}, err
	}
	defer db.Close()

	if reqBody.Type == "post" {
		encodedId, err := shortenUrl(reqBody.Value, db)
		if err != nil {
			return events.APIGatewayProxyResponse{
				StatusCode:      400,
				Body:            fmt.Sprintf("%v", err),
				Headers:         headers,
				IsBase64Encoded: false,
			}, err
		}

		return events.APIGatewayProxyResponse{
			StatusCode:      200,
			Body:            fmt.Sprintf("{\"id\": \"%v\"}", encodedId),
			Headers:         headers,
			IsBase64Encoded: false,
		}, nil

	} else if reqBody.Type == "get" {

		u, err := getRow(reqBody.Value, db)
		if err != nil {
			return events.APIGatewayProxyResponse{
				StatusCode:      400,
				Body:            string("Failed to get row"),
				Headers:         headers,
				IsBase64Encoded: false,
			}, err
		}

		return events.APIGatewayProxyResponse{
			StatusCode:      200,
			Body:            fmt.Sprintf("{\"id\": \"%v\"}", u),
			Headers:         headers,
			IsBase64Encoded: false,
		}, nil

	}

	return events.APIGatewayProxyResponse{
		StatusCode:      200,
		Body:            "Invalid method",
		IsBase64Encoded: false,
	}, err

}

func shortenUrl(link string, db *sql.DB) (string, error) {
	GOOGLEAPIKEY := os.Getenv("GOOGLE_API_KEY")
	err := validUrl(link)
	if err != nil {
		return "", err
	}

	log.Println("Link is valid")
	finalLink, err := getFinalUrl(link)
	if err != nil {
		return "", err
	}

	log.Println("Got final link:", finalLink)

	isMalicious, err := checkUrlMalicious(finalLink, GOOGLEAPIKEY)
	if err != nil {
		return "", err
	}

	log.Println("Link is mal:", isMalicious)

	if isMalicious {
		return "", err
	}

	// Add to db
	id, err := insertUrl(finalLink, db)
	if err != nil {
		return "", err
	}

	log.Println("Added to db", id)
	encodedId := encodeId(id)

	log.Println("Encoded id", encodedId)
	err = updateRow(id, encodedId, db)
	if err != nil {
		return "", err
	}

	log.Println("Updated row")

	return encodedId, nil
}

func connect(username string, password string, host string, database string, port string) (*sql.DB, error) {

	// TODO do full-verify instead of require
	// connection := fmt.Sprintf("postgres://%v:%v@%v:%v/%v?sslmode=require", username, password, host, port, database)
	connection := fmt.Sprintf("dbname=%v user=%v password=%v host=%v port=%v sslmode=require binary_parameters=yes", database, username, password, host, port)

	db, err := sql.Open("postgres", connection)
	if err != nil {
		fmt.Println("Error connecting to postgres", err)
		return nil, err
	}

	return db, nil
}

func validUrl(link string) error {
	_, err := url.ParseRequestURI(link)
	if err != nil {
		return err
	}
	return nil
}

// Takes in a url and follows it until the very end, done to avoid b.s. tracking/affiliate links
func getFinalUrl(link string) (string, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", link, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:121.0) Gecko/20100101 Firefox/121.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	// req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("DNT", "1")
	req.Header.Set("Sec-GPC", "1")
	req.Header.Set("Connection", "keep-alive")
	// req.Header.Set("Cookie", "ax=v412-3; ay=b")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()

	return res.Request.URL.String(), nil
}

// Checks to make sure a url is not a malicious link.
// !TODO: read what the body says
func checkUrlMalicious(link string, apiKey string) (bool, error) {

	payload := strings.NewReader(fmt.Sprintf("{\n  \"client\": {\n    \"clientId\": \"flusterShortener\",\n    \"clientVersion\": \"0.0.1\"\n  },\n  \"threatInfo\": {\n    \"threatTypes\": [\n      \"MALWARE\",\n      \"SOCIAL_ENGINEERING\",\n      \"POTENTIALLY_HARMFUL_APPLICATION\",\n      \"UNWANTED_SOFTWARE\",\n      ],\n    \"platformTypes\": [\n      \"ANY_PLATFORM\"\n    ],\n    \"threatEntryTypes\": [\n      \"URL\"\n    ],\n    \"threatEntries\": [\n      {\n        \"url\": \"%v\"\n      }\n    ]\n  }\n}\n", link))

	req, err := http.NewRequest("POST", fmt.Sprintf("https://safebrowsing.googleapis.com/v4/threatMatches:find?key=%v", apiKey), payload)
	if err != nil {
		return false, err
	}

	req.Header.Add("Content-Type", "application/json")
	// req.Header.Add("User-Agent", "insomnia/8.5.1")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return false, err
	}
	// log.Println(string(body))
	if len(body) != 3 {
		return true, nil
	}

	return false, nil
}

func testSelect(db *sql.DB) error {
	str := "SELECT * from urls"
	_, err := db.Query(str)
	if err != nil {
		return err
	}
	return nil
}

// Inserts the url, returning id so we can encode it
func insertUrl(link string, db *sql.DB) (int, error) {
	str := "INSERT INTO urls (url) VALUES ($1) returning id;"

	row, err := db.Query(str, link)
	if err != nil {
		return 0, err
	}
	defer row.Close()

	id := 0
	for row.Next() {
		row.Scan(&id)
	}

	if row.Err() != nil {
		return 0, nil
	}

	return id, nil
}

// Update the row id with the encoded value
func updateRow(id int, encode string, db *sql.DB) error {
	str := `UPDATE urls SET encode = $1 WHERE id = $2 returning id, encode;`
	row, err := db.Query(str, encode, id)
	if err != nil {
		return err
	}
	defer row.Close()

	i := 0
	e := ""
	for row.Next() {
		row.Scan(&i, &e)
	}

	if row.Err() != nil {
		return err
	}

	return nil
}

func getRow(encodedId string, db *sql.DB) (string, error) {
	str := `SELECT url FROM urls WHERE encode = $1`
	row, err := db.Query(str, encodedId)
	if err != nil {
		return "", err
	}
	defer row.Close()

	u := ""
	for row.Next() {
		row.Scan(&u)
	}

	if row.Err() != nil {
		return "", err
	}

	if u == "" {
		return "", errors.New("url is blank")
	}

	return u, nil
}

// Encode an id
func encodeId(id int) string {
	data := []byte(fmt.Sprintf("%v", id))
	return base58.Encode(data)
}

// Decode an encoded string into int
func decodeId(encodedVal string) (int, error) {
	dec := base58.Decode(encodedVal)
	id, err := strconv.Atoi(string(dec))
	if err != nil {
		return 0, err
	}
	return id, nil
}
