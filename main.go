package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/goombaio/namegenerator"
	"github.com/gorilla/websocket"

	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"time"
)

var addr = flag.String("addr", "127.0.0.1:8000", "http service address")

var httpPrefix = "http://"

var basicAuthUser = "test"
var basicAuthPass = "test"

type UserStruct struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type PatientPageList struct {
	Count int `json:"count"`
	//Next     int       `json:"next"`
	//Previous int       `json:"previous"`
	Results []Patient `json:"results"`
}

type Patient struct {
	Name       string     `json:"name"`
	Age        int        `json:"age"`
	RoomNumber int        `json:"room_no"` //fixme: this should not be int
	Gender     string     `json:"gender"`
	DeviceID   string     `json:"device_id"`
	DeviceType string     `json:"device_type"`
	User       UserStruct `json:"user"`
}

type DataPacket struct {
	Command  string `json:"command"`
	DeviceID string `json:"device_id"`
	SeqID    int    `json:"sequence_id"`
	Time     int64  `json:"time"`
	Value    int    `json:"value"`
	Battery  int    `json:"battery"`
}

func userPost(URL string, user string, passwd string, data []byte) (*http.Response, error) {
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodPost, URL, bytes.NewReader(data))
	//req.SetBasicAuth(user, passwd)
	req.Header.Set("Authorization", "Token 79bfff7c4e78a575af2226fde003609680112e85")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	bodyText, err := ioutil.ReadAll(resp.Body)
	s := string(bodyText)
	fmt.Println(s)

	return resp, err
}

func getUserList() []Patient {
	client := &http.Client{}
	URL := httpPrefix + *addr + "/seniors/"
	req, err := http.NewRequest(http.MethodGet, URL, nil)
	//req.SetBasicAuth(basicAuthUser, basicAuthPass)
	req.Header.Set("Authorization", "Token 79bfff7c4e78a575af2226fde003609680112e85")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	//spew.Dump(resp.Body)
	bodyText, err := ioutil.ReadAll(resp.Body)
	var pList PatientPageList
	err = json.Unmarshal(bodyText, &pList)
	if err != nil {
		log.Fatal(err)
	}
	return pList.Results
}

func deleteUser(deviceID string) {
	client := &http.Client{}
	URL := httpPrefix + *addr + "/seniors/" + deviceID + "/" //https://docs.djangoproject.com/en/2.2/ref/settings/#append-slash
	fmt.Println(URL)

	req, err := http.NewRequest(http.MethodDelete, URL, nil)
	//req.SetBasicAuth(basicAuthUser, basicAuthPass)
	req.Header.Set("Authorization", "Token 79bfff7c4e78a575af2226fde003609680112e85")
	//req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	// Read Response Body
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(string(respBody))
}

func websocketSend(deviceID string) {

	dataPacket := DataPacket{"new", deviceID, 0, time.Now().UnixNano() / 1000000, 12, 60}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{Scheme: "ws", Host: *addr, Path: "/ws/sensor/RR"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			log.Printf("recv: %s", message)
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			sigData, err := json.Marshal(dataPacket)
			if err != nil {
				log.Fatal(err)
			}
			err = c.WriteMessage(websocket.TextMessage, sigData)
			if err != nil {
				log.Println("write:", err)
				return
			}
			dataPacket.SeqID = dataPacket.SeqID + 1
			dataPacket.Time = time.Now().UnixNano() / 1000000
			dataPacket.Value = rand.Intn(100-80+1) + 80
			dataPacket.Command = "update"
		case <-interrupt:
			log.Println("interrupt")
			deleteUser("7C1A23F227B4")
			dataPacket.Command = "close"
			sigData, err := json.Marshal(dataPacket)
			if err != nil {
				log.Fatal(err)
			}
			err = c.WriteMessage(websocket.TextMessage, sigData)
			if err != nil {
				log.Println("write close1:", err)
				return
			}
			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err = c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close2:", err)
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}

func createUser() string {
	patientName := NameGenerator.Generate()
	deviceID := RandStringRunes(12)
	userData, err := json.Marshal(Patient{Name: patientName, Age: 31, RoomNumber: 1, Gender: "M", DeviceID: deviceID, DeviceType: "RRI", User: UserStruct{Username: RandStringRunes(6), Email: "test@test.com", Password: "test3"}})
	if err != nil {
		log.Fatal(err)
	}

	resp, err := userPost(httpPrefix+*addr+"/seniors/", basicAuthUser, basicAuthPass, userData)
	if err != nil {
		fmt.Println(err)
		log.Fatal(err)
	}

	var res map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&res)
	fmt.Println(res["json"])
	return deviceID
}

var CommandFlag string
var NameGenerator namegenerator.Generator

func init() {
	flag.StringVar(&CommandFlag, "c", "user", "command for run goclient")
	rand.Seed(time.Now().UnixNano())
	seed := time.Now().UTC().UnixNano()
	NameGenerator = namegenerator.NewNameGenerator(seed)

}

var letterRunes = []rune("1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ")

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func main() {
	flag.Parse()
	log.SetFlags(0)

	var deviceId string

	if CommandFlag == "user" {
		deviceId = createUser()
	} else if CommandFlag == "clear" {
		userList := getUserList()
		for _, val := range userList {
			fmt.Println("deleting user: ", val)
			deleteUser(val.DeviceID)
		}
		//deleteUser("7C1A23F227B4")
	} else if CommandFlag == "ws" {
		deviceId = createUser()
		websocketSend(deviceId)
	}

}
