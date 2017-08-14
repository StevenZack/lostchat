package main

import (
	"crypto/md5"
	"fmt"
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
	fmt.Println(sid)
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
	case "getAvatar":
		http.ServeFile(w, r, "./public/avatars/default")
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
