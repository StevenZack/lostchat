package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"golang.org/x/net/websocket"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"html/template"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"time"
)

var (
	MongoDBServer string = "127.0.0.1"
)

type User struct {
	Email, Password, SessionID     string
	Name, AttireID, HomeID, Avatar string
	Online                         string
	Money                          int
}
type Friend struct {
	From, To string
	Remark   string
}

func main() {
	http.HandleFunc("/", home)
	http.HandleFunc("/login", login)
	http.HandleFunc("/jsonreq/", jsonreq)
	http.Handle("/connection", websocket.Handler(wshandler))
	http.Handle("/public/", http.StripPrefix("/public/", http.FileServer(http.Dir("./public"))))
	err := http.ListenAndServe(":8090", nil)
	if err != nil {
		fmt.Println(err)
	}
}
func home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		t, _ := template.ParseFiles("notfound.html")
		t.Execute(w, nil)
		return
	}
	sid, err := r.Cookie("lostchat-sessionid")
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	s, err := mgo.Dial(MongoDBServer)
	if err != nil {
		go RestartMongodb()
		fmt.Println(err)
		return
	}
	defer s.Close()
	cu := s.DB("lostchat").C("users")
	cf := s.DB("lostchat").C("friends")

	type HomeData struct {
		Me      User
		Friends []Friend
	}
	hd := HomeData{}
	err = cu.Find(bson.M{"sessionid": sid.Value}).One(&hd.Me)
	if err != nil {
		http.SetCookie(w, &http.Cookie{Name: "lostchat-sessionid", Value: "", Expires: time.Now()})
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	err = cf.Find(bson.M{"from": hd.Me.Email}).All(&hd.Friends)
	if err != nil {
		fmt.Println(err)
		return
	}
	t, err := template.ParseFiles("index.html")
	if err != nil {
		fmt.Println(err)
		return
	}
	t.Execute(w, hd)
}
func login(w http.ResponseWriter, r *http.Request) {
	type Info struct {
		Info string
	}
	_, err := r.Cookie("lostchat-sessionid")
	if err == nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	email := r.FormValue("Email")
	if r.Method == "GET" || email == "" {
		t, err := template.ParseFiles("login.html")
		if err != nil {
			fmt.Println(err)
			return
		}
		t.Execute(w, nil)
		return
	}
	state := r.FormValue("State")
	password := r.FormValue("Password")
	s, err := mgo.Dial(MongoDBServer)
	if err != nil {
		go RestartMongodb()
		fmt.Println(err)
		return
	}
	defer s.Close()
	cu := s.DB("lostchat").C("users")
	if state == "REGISTER" {
		emailc, _ := cu.Find(bson.M{"email": email}).Count()
		if emailc > 0 {
			t, err := template.ParseFiles("login.html")
			if err != nil {
				fmt.Println(err)
				return
			}
			t.Execute(w, Info{Info: "账号已存在"})
			return
		}
		name := r.FormValue("Name")
		sid := NewToken()
		err = cu.Insert(User{
			Email: email, Password: password, SessionID: sid,
			Name: name, AttireID: "default", HomeID: "default", Avatar: "default.webp",
			Online: "no", Money: 0,
		})
		if err != nil {
			fmt.Println(err)
			return
		}
		err = exec.Command("cp", "./public/avatars/default", "./public/avatars/"+email).Run()
		if err != nil {
			fmt.Println("cp avatar failed", err)
			return
		}
		err = exec.Command("cp", "-r", "./public/homes/default", "./public/homes/"+email).Run()
		if err != nil {
			fmt.Println("cp home failed", err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "lostchat-sessionid", Value: sid, Expires: time.Now().AddDate(1, 0, 0)})
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	u := User{}
	err = cu.Find(bson.M{"email": email}).One(&u)
	if err != nil {
		t, _ := template.ParseFiles("login.html")
		t.Execute(w, Info{Info: "账号不存在，请先注册"})
		return
	}
	if password != u.Password {
		t, _ := template.ParseFiles("login.html")
		t.Execute(w, Info{Info: "密码不正确"})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "lostchat-sessionid", Value: u.SessionID, Expires: time.Now().AddDate(1, 0, 0)})
	http.Redirect(w, r, "/", http.StatusFound)
}
func jsonreq(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	fmt.Println(r.URL.Path)
	str := r.URL.Path[len("/jsonreq/"):]
	switch str {
	case "checkOnline":
		if ConMap[r.Form["Email"][0]] != nil {
			fmt.Fprint(w, "true")
			return
		}
		fmt.Fprint(w, "false")
		return
	case "addFriend":
		email := r.Form["Email"][0]
		me := r.Form["Me"][0]
		s, err := mgo.Dial(MongoDBServer)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer s.Close()
		cu := s.DB("lostchat").C("users")
		cf := s.DB("lostchat").C("friends")
		emailC, err := cu.Find(bson.M{"email": email}).Count()
		if err != nil || emailC < 1 {
			fmt.Fprint(w, "没有这个用户")
			return
		}
		u := User{}
		err = cu.Find(bson.M{"sessionid": me}).One(&u)
		if err != nil {
			fmt.Fprint(w, "登录信息失效,请重新登录")
			return
		}
		fC, err := cf.Find(bson.M{"from": u.Email, "to": email}).Count()
		if err == nil && fC > 0 {
			fmt.Fprint(w, "已经是好友")
			return
		}
		err = cf.Insert(Friend{From: u.Email, To: email})
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Fprint(w, "OK")
		return
	case "deleteFriend":
		email := r.Form["Email"][0]
		me := r.Form["Me"][0]
		s, err := mgo.Dial(MongoDBServer)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer s.Close()
		cu := s.DB("lostchat").C("users")
		cf := s.DB("lostchat").C("friends")
		emailC, err := cu.Find(bson.M{"email": email}).Count()
		if err != nil || emailC < 1 {
			fmt.Fprint(w, "没有这个用户")
			return
		}
		u := User{}
		err = cu.Find(bson.M{"sessionid": me}).One(&u)
		if err != nil {
			fmt.Fprint(w, "登录信息失效,请重新登录")
			return
		}
		fC, err := cf.Find(bson.M{"from": u.Email, "to": email}).Count()
		if err != nil || fC < 1 {
			fmt.Fprint(w, "不是好友关系")
			return
		}
		err = cf.Remove(bson.M{"from": u.Email, "to": email})
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Fprint(w, "OK")
		return
	case "setRemark":
		email := r.Form["Email"][0]
		me := r.Form["Me"][0]
		remark := r.Form["Remark"][0]
		s, err := mgo.Dial(MongoDBServer)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer s.Close()
		cu := s.DB("lostchat").C("users")
		cf := s.DB("lostchat").C("friends")
		emailC, err := cu.Find(bson.M{"email": email}).Count()
		if err != nil || emailC < 1 {
			fmt.Fprint(w, "没有这个用户")
			return
		}
		u := User{}
		err = cu.Find(bson.M{"sessionid": me}).One(&u)
		if err != nil {
			fmt.Fprint(w, "登录信息失效,请重新登录")
			return
		}
		fC, err := cf.Find(bson.M{"from": u.Email, "to": email}).Count()
		if err != nil || fC < 1 {
			fmt.Fprint(w, "不是好友关系")
			return
		}
		err = cf.Update(bson.M{"from": u.Email, "to": email}, bson.M{"$set": bson.M{"remark": remark}})
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Fprint(w, "OK")
		return
	}
}
func RestartMongodb() {
	exec.Command("systemctl", "restart", "mongodb").Run()
}
func NewToken() string {
	crutime := time.Now().Unix()
	h := md5.New()
	io.WriteString(h, strconv.FormatInt(crutime, 10))
	token := fmt.Sprintf("%x", h.Sum(nil))
	return token
}

var (
	ConMap = make(map[string]*websocket.Conn)
)

type BaseInfo struct {
	State, Info string
}
type Msg struct {
	BaseInfo
	Text               string
	AttireID           string
	Action             string
	FromEmail, ToEmail string
}

func wshandler(ws *websocket.Conn) {
	defer ws.Close()
	defer fmt.Println("ws closed")
	bi := BaseInfo{}
	b := make([]byte, 128)
	length, err := ws.Read(b)
	if err != nil {
		return
	}
	err = json.Unmarshal(b[:length], &bi)
	if err != nil || bi.State != "SessionID" {
		ReturnInfo(ws, "ERR", "Protocol mismatch")
		return
	}
	s, err := mgo.Dial(MongoDBServer)
	if err != nil {
		go RestartMongodb()
		ReturnInfo(ws, "SERVER-ERR", "cannot connect to MongoDB")
		return
	}
	defer s.Close()
	cu := s.DB("lostchat").C("users")
	u := User{}
	err = cu.Find(bson.M{"sessionid": bi.Info}).One(&u)
	if err != nil {
		ReturnInfo(ws, "ERR", "SessionID out of date")
		return
	}
	ConMap[u.Email] = ws
	ReturnInfo(ws, "OK", "Succeed")
LoopFlag:
	for {
		length, err = ws.Read(b)
		if err != nil {
			ConMap[u.Email] = nil
			return
		}
		msg := Msg{}
		err = json.Unmarshal(b[:length], &msg)
		if err != nil {
			continue
		}
		switch msg.State {
		case "SEND":
			if ConMap[msg.ToEmail] != nil {
				msg.FromEmail = u.Email
				ReturnData(ConMap[msg.ToEmail], msg)
				msg.State = "SENT"
				ReturnData(ws, msg)
				continue LoopFlag
			}
			msg.State = "UNSENT"
			ReturnData(ws, msg)
			continue LoopFlag
		}
	}
}
func ReturnInfo(w io.Writer, state, info string) {
	b := BaseInfo{State: state, Info: info}
	d, _ := json.Marshal(b)
	w.Write(d)
}
func ReturnData(w io.Writer, data interface{}) {
	d, _ := json.Marshal(data)
	w.Write(d)
}
