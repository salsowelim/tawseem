/*Copyright (c) 2019, Suliman Alsowelim
All rights reserved.
This source code is licensed under the MIT license found in the
LICENSE file in the root directory of this source tree.
*/
package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"html/template"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"time"
)

const No_OF_ACTIVE_USERS_LIMIT = 1000

var TimersMap = map[string]*Timers{}
var activeUsers = map[string]*ActiveUser{}
var db *sql.DB
var db_file_path, templates_directory string
var parse_form_function func(*sql.DB, *http.Request, string, int, int) (bool, string) // variable to hold function
var exitchannel chan bool

type ActiveUser struct {
	session       string // session id
	c_word        string // current word
	accomplished  int    // number of words accomplished
	total_words   int    // total number of words in the assigned text
	text_id       int    // the assigned text_id
	word_seq      int    // word id that is being worked on currently
	display_error bool   // should display error msg next time the active user access the site
	error_msg     string // the error msg
	bulkText      string // the text itself
	timeout       int    // remove user session if timeouts
}
type Timers struct {
	timeout int
}
type mainTemplateData struct {
	Id       int           // id of the text being worked on
	Word     template.HTML // ready to process word
	Percent  string        // (processed words/total number of words in text) *100
	IsError  bool          // should display error?
	ErrorMsg string
}
type loginTemplateData struct {
	IsError  bool
	ErrorMsg string
}
type bulktext_word struct {
	word      string
	processed int // flag to indicate the word status: 0 not processed, 1 processed, 2 pre-processed
}

type FormParser struct {
}

/*
	if session active --> redirect to main page.
	get request --> provide login page
	post request --> verify,create session, and redirect to main
*/
func loginHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("nlp_platform")
	if err == nil {
		user := activeUsers[cookie.Value]
		if user != nil {
			setsessionTimeout(user)
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		cookies := http.Cookie{Name: "nlp_platform", Path: "/", HttpOnly: true, Expires: time.Unix(0, 0)}
		http.SetCookie(w, &cookies)
	}
	if r.Method == "GET" {
		t, _ := template.ParseFiles(templates_directory+"/login.html", templates_directory+"/header_visitor.html", templates_directory+"/footer.html")
		t.Execute(w, nil)
	} else if r.Method == "POST" {
		r.ParseForm()
		authorized := false
		var err error
		uid := -1
		_, ok := r.Form["inputuserid"]
		_, ok2 := r.Form["inputusername"]
		if ok && ok2 {
			authorized = true
		}

		if authorized {
			uid, err = strconv.Atoi(r.Form["inputuserid"][0])
			if err != nil {
				authorized = false
			} else {
				authorized = dbCheckUser(db, uid, r.Form["inputusername"][0])
			}
		}
		if !authorized {
			// incorrect username or userid
			temp := loginTemplateData{
				IsError: true,
				ErrorMsg: `الاسم أو رقم المستخدم غير صحيح. الرجاء إعادة المحاولة. 
				Username or ID is not correct. Please try again.`,
			}
			t, _ := template.ParseFiles(templates_directory+"/login.html", templates_directory+"/header_visitor.html", templates_directory+"/footer.html")
			t.Execute(w, temp)
			return
		}
		if len(activeUsers) >= No_OF_ACTIVE_USERS_LIMIT-1 {
			// server is full
			temp := loginTemplateData{
				IsError: true,
				ErrorMsg: `الموقع يعاني حاليا من تزايد عدد المستخدمين. رجاءا أعد المحاولة في وقت لاحق. 
				Server now is full. Please try to login later.`,
			}
			t, _ := template.ParseFiles(templates_directory+"/login.html", templates_directory+"/header_visitor.html", templates_directory+"/footer.html")
			t.Execute(w, temp)
			return
		}
		tmp_text_id := dbWorksOn(db, uid)
		if tmp_text_id == -1 {
			// no more text error
			temp := loginTemplateData{
				IsError: true,
				ErrorMsg: `لا يوجد نص جاهز حاليا وقابل للتوسيم. الرجاء المحاولة لاحقا. 
				There is no text now ready for processing. Please check later. `,
			}
			t, _ := template.ParseFiles(templates_directory+"/login.html", templates_directory+"/header_visitor.html", templates_directory+"/footer.html")
			t.Execute(w, temp)
			return
		}
		tmp_bulkText := dbGetText(db, tmp_text_id)
		tmp_total_words := dbGetTotalWords(db, tmp_text_id)
		userdata := &ActiveUser{
			session:       randomToken(9),
			accomplished:  0,
			total_words:   tmp_total_words,
			text_id:       tmp_text_id,
			word_seq:      0,
			display_error: false,
			bulkText:      tmp_bulkText,
		}
		activeUsers[userdata.session] = userdata
		setsessionTimeout(activeUsers[userdata.session])
		cookies := http.Cookie{Name: "nlp_platform", Value: userdata.session, Path: "/", /*Secure: true,*/
			HttpOnly: true}
		http.SetCookie(w, &cookies)
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

/*
	- if user not logged in, or session not registered in cache --> redirect to login page.
	- if get request -- provide new word to process
	- if post request -- add processed word to db - extend session - redirect to main -
*/
func mainHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("nlp_platform")
	if err == nil {
		user := activeUsers[cookie.Value]
		if user == nil {
			// not active user, need to relogin
			http.Redirect(w, r, "/login/", http.StatusFound)
			return
		}
	} else {
		// user not logged in
		http.Redirect(w, r, "/login/", http.StatusFound)
		return
	}
	if r.Method == "GET" {
		setsessionTimeout(activeUsers[cookie.Value])
		accomplishedWords := dbGetAccomplished(db, activeUsers[cookie.Value].text_id)
		remaining_words := activeUsers[cookie.Value].total_words - accomplishedWords
		if remaining_words == 0 {
			// -- update db --  thank user -- destroy session
			dbUpdateBulk(db, activeUsers[cookie.Value].text_id)
			t, _ := template.ParseFiles(templates_directory+"/patchc.html", templates_directory+"/header_visitor.html", templates_directory+"/footer.html")
			errr := t.Execute(w, nil)
			checkError(errr)
			delete(activeUsers, cookie.Value)
			cookies := http.Cookie{Name: "nlp_platform", Path: "/", HttpOnly: true, Expires: time.Unix(0, 0)}
			http.SetCookie(w, &cookies)
			return
		}
		word_seq := dbGetUnprocessedWord(db, activeUsers[cookie.Value].text_id)
		activeUsers[cookie.Value].word_seq = word_seq // first unprocessed word index
		wordslist := make([]bulktext_word, activeUsers[cookie.Value].total_words)
		// lazy way to load words, but anyway...
		dbLoadWordsFromText(db, activeUsers[cookie.Value].text_id, wordslist)
		tmpstring := ""
		for i := 0; i < len(wordslist); i++ {
			if word_seq == i+1 {
				// we found our word
				activeUsers[cookie.Value].c_word = wordslist[i].word
				tmpstring += " <font  color=\"red\" style=\"font-size:120%;\"><b  id=\"theword\"> "
				tmpstring += wordslist[i].word
				tmpstring += " </b> </font> "
				continue
			}
			if wordslist[i].processed == 2 {
				//pre_segmented
				tmpstring += " <font color=\"#247a00\" style=\"font-size:105%;\"> "
				tmpstring += wordslist[i].word
				tmpstring += " </font> "
				continue
			}

			if wordslist[i].processed == 1 {
				//already processed by user
				tmpstring += " <font color=\"#247a00\" style=\"font-size:105%;\"> "
				tmpstring += wordslist[i].word
				tmpstring += " </font> "
				continue
			}
			tmpstring += wordslist[i].word
			tmpstring += " "
		}
		tmp := (float64)(accomplishedWords)
		tmp2 := (float64)(activeUsers[cookie.Value].total_words)
		a := (tmp / tmp2) * 100
		tmp_percent := strconv.Itoa(int(a))
		var t *template.Template
		var err error
		temp := mainTemplateData{
			Id:       activeUsers[cookie.Value].text_id,
			Word:     template.HTML(tmpstring),
			Percent:  tmp_percent,
			IsError:  activeUsers[cookie.Value].display_error,
			ErrorMsg: activeUsers[cookie.Value].error_msg,
		}

		t, err = template.ParseFiles(templates_directory+"/main.html", templates_directory+"/header.html", templates_directory+"/footer.html", templates_directory+"/user_form.html")
		checkError(err)
		errr := t.Execute(w, temp)
		checkError(errr)
	} else if r.Method == "POST" {
		//add to db - update local session - redirect to main -
		setsessionTimeout(activeUsers[cookie.Value])
		r.ParseForm()
		if _, ok2 := r.Form["goback"]; ok2 {
			dbGoBack(db, activeUsers[cookie.Value].text_id)
		} else {
			activeUsers[cookie.Value].display_error, activeUsers[cookie.Value].error_msg = parse_form_function(db, r, activeUsers[cookie.Value].c_word,
				activeUsers[cookie.Value].word_seq, activeUsers[cookie.Value].text_id)

		}
		http.Redirect(w, r, "/", http.StatusFound)
	}
}
func checkError(err error) {
	/* src https://jan.newmarch.name/go/template/chapter-template.html*/
	if err != nil {
		fmt.Println("Fatal error ", err.Error())
		os.Exit(1)
	}
}
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	// destroy session
	cookie, err := r.Cookie("nlp_platform")
	if err == nil {
		user := activeUsers[cookie.Value]
		if user != nil {
			delete(activeUsers, cookie.Value)
		}
		cookies := http.Cookie{Name: "nlp_platform", Path: "/", HttpOnly: true, Expires: time.Unix(0, 0)}
		http.SetCookie(w, &cookies)
	}
	http.Redirect(w, r, "/login/", http.StatusFound)
}

func helpHandler(w http.ResponseWriter, r *http.Request) {
	t, _ := template.ParseFiles(templates_directory+"/help.html", templates_directory+"/header.html", templates_directory+"/footer.html")
	errr := t.Execute(w, nil)
	checkError(errr)
}
func setsessionTimeout(user *ActiveUser) {
	user.timeout++
	if user.timeout > 1000 {
		user.timeout = 0
	}
}

// Goroutine that runs when server is alive. It iterate through active users lists and remove inactive sessions.
func timer(shouldexit chan bool) {
	shouldloop := true
	for shouldloop {
		select {
		case <-shouldexit:
			shouldloop = false
			break
		case <-time.After(900 * time.Second):
			for sessionID := range activeUsers {
				if TimersMap[sessionID] != nil {
					if activeUsers[sessionID].timeout == TimersMap[sessionID].timeout {
						delete(activeUsers, sessionID)
						delete(TimersMap, sessionID)
					} else {
						TimersMap[sessionID].timeout = activeUsers[sessionID].timeout
					}
				} else {
					TimersMap[sessionID] = &Timers{0}
					TimersMap[sessionID].timeout = 0
				}
			}
		}
	}
	return
}
func main() {
	args := os.Args
	if len(args) != 4 {
		fmt.Println("Please provide only those three arguments (in order):\n 1.sqlite file path\n 2.path to template files directory\n 3.name of the chosen function to parse user input")
		os.Exit(1)
	}
	db_file_path = args[1]
	templates_directory = args[2]
	//https://stackoverflow.com/questions/20714939/how-to-properly-use-call-in-reflect-package
	p := &FormParser{}
	methodVal := reflect.ValueOf(p).MethodByName(args[3])
	methodIface := methodVal.Interface()
	parse_form_function = methodIface.(func(*sql.DB, *http.Request, string, int, int) (bool, string))
	exitchannel = make(chan bool)
	go timer(exitchannel)
	db = NewDB()
	t := time.Now()
	fmt.Println(t.Format("2006/01/02:15:04:05") + "[starting server]")
	activeUsers = make(map[string]*ActiveUser, No_OF_ACTIVE_USERS_LIMIT)
	TimersMap = make(map[string]*Timers, No_OF_ACTIVE_USERS_LIMIT)
	http.HandleFunc("/login/", loginHandler)
	http.HandleFunc("/help/", helpHandler)
	http.HandleFunc("/logout/", logoutHandler)
	http.HandleFunc("/", mainHandler)
	//http://stackoverflow.com/questions/13302020/rendering-css-in-a-go-web-application
	fs := justFilesFilesystem{http.Dir("static/")}
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(fs)))
	http.ListenAndServe(":8080", nil)
}

type justFilesFilesystem struct {
	fs http.FileSystem
}

func (fs justFilesFilesystem) Open(name string) (http.File, error) {
	f, err := fs.fs.Open(name)
	if err != nil {
		return nil, err
	}
	return neuteredReaddirFile{f}, nil
}

type neuteredReaddirFile struct {
	http.File
}

func (f neuteredReaddirFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, nil
}

func NewDB() *sql.DB {
	db, err := sql.Open("sqlite3", db_file_path)
	db.SetConnMaxLifetime(time.Second * 14400)
	if err != nil {
		panic(err)
	}
	return db
}

/*
Get user form input from http.request, and add it to db. return bool flag (true if there is error), and string explains
the error to user, if any.
function input:
db => pointer to our database
r => user http request
word => the word that is being processed by user
w_seq => word index
text_id => id of the text that is being processed by user
*/
func (p *FormParser) Parse_seg(db *sql.DB, r *http.Request, word string, w_seq int, text_id int) (bool, string) {
	isSegmented := r.Form["isSegmented"][0]
	var stm string
	if isSegmented == "yes" {
		stm = word
	} else if isSegmented == "no" {
		stm = r.Form["stm"][0]
		is_a_segmented := false
		is_b_segmented := false
		if _, ok := r.Form["a_segmented"]; ok {
			is_a_segmented = true
		}
		if _, ok2 := r.Form["b_segmented"]; ok2 {
			is_b_segmented = true
		}

		if !is_a_segmented && !is_b_segmented {
			return true, "تأكد من اختيارك : هل هناك إضافات سابقة أو لاحقة؟"
		}
		if is_a_segmented {
			a_no_s := r.Form["a_no_s"][0]
			if a_no_s == "0" {
				return true, "تأكد من ادخال العدد الصحيح من الاضافات اللاحقة"
			}
			switch a_no_s {
			case "2view4":
				if r.Form["a_4_s"][0] == "" {
					return true, "تأكد من تحديد الإضافة اللاحقة الرابعة"
				}
				fallthrough
			case "2view3":
				if r.Form["a_3_s"][0] == "" {
					return true, "تأكد من تحديد الإضافة اللاحقة الثالثة"
				}
				fallthrough
			case "2view2":
				if r.Form["a_2_s"][0] == "" {
					return true, "تأكد من تحديد الإضافة اللاحقة الثانية"
				}
				fallthrough
			case "2view1":
				if r.Form["a_1_s"][0] == "" {
					return true, "تأكد من تحديد الإضافة اللاحقة الأولى"
				}
			default:
				return true, "تأكد من تحديد عدد الإضافة اللاحقة بشكل صحيح"
			}
		}

		if is_b_segmented {
			b_no_s := r.Form["b_no_s"][0]
			if b_no_s == "0" {
				return true, "تأكد من ادخال العدد الصحيح من الاضافات السابقة"
			}
			switch b_no_s {
			case "view4":
				if r.Form["b_4_s"][0] == "" {
					return true, "تأكد من تحديد الإضافة السابقة الرابعة"
				}
				fallthrough
			case "view3":
				if r.Form["b_3_s"][0] == "" {
					return true, "تأكد من تحديد الإضافة السابقة الثالثة"
				}
				fallthrough
			case "view2":
				if r.Form["b_2_s"][0] == "" {
					return true, "تأكد من تحديد الإضافة السابقة الثانية"
				}
				fallthrough
			case "view1":
				if r.Form["b_1_s"][0] == "" {
					return true, "تأكد من تحديد الإضافة السابقة الأولى"
				}
			default:
				return true, "تأكد من تحديد عدد الإضافة السابقة بشكل صحيح"
			}
		}
	} else {
		return true, "تأكد من البيانات المدخلة"
	}
	_, err := db.Exec(`INSERT OR REPLACE INTO  words
   (w_seq, word, pr1, pr2, pr3, pr4, stm, sf1, sf2, sf3, sf4,
    processed, text_id) VALUES (
    ?, ?, ?,?,?,?,?,?,?,?,?,?,?)`,
		w_seq, word, r.Form["b_1_s"][0], r.Form["b_2_s"][0], r.Form["b_3_s"][0],
		r.Form["b_4_s"][0], stm, r.Form["a_1_s"][0], r.Form["a_2_s"][0], r.Form["a_3_s"][0],
		r.Form["a_4_s"][0], 1, text_id)
	if err != nil {
		panic(err)
	}
	return false, ""
}

func (p *FormParser) Parse_pos(db *sql.DB, r *http.Request, word string, w_seq int, text_id int) (bool, string) {
	tag_selector := r.Form["tagselector"][0]
	if tag_selector == "0" {
		return true, "Please choose a tag from the dropdown list"
	}
	_, err := db.Exec(`INSERT OR REPLACE INTO  words
   (w_seq, word,tag,processed, text_id) VALUES (
    ?,?,?,?,?)`,
		w_seq, word, tag_selector, 1, text_id)
	if err != nil {
		panic(err)
	}
	return false, ""
}

func dbGetText(db *sql.DB, t_id int) string {
	var text string
	err := db.QueryRow("select content from texts where t_id = ?", t_id).Scan(&text)
	if err != nil {
		panic(err)
	}
	return text
}

/* load words of text with t_id into our array/slice, along with processed flag */
func dbLoadWordsFromText(db *sql.DB, t_id int, wordlist []bulktext_word) {
	var s sql.NullInt64
	rows, err := db.Query("select word,processed from words where text_id = ? ORDER BY w_seq", t_id)
	if err != nil {
		panic(err)
	}
	i := 0
	defer rows.Close()
	for rows.Next() {
		err3 := rows.Scan(&wordlist[i].word, &s)
		if s.Valid {
			wordlist[i].processed = int(s.Int64)
		} else {
			wordlist[i].processed = 0
		}
		i = i + 1
		if err3 != nil {
			panic(err3)
		}
	}
}

/* get first unprocessed word index */
func dbGetUnprocessedWord(db *sql.DB, t_id int) int {
	var min_seq int
	var err error
	err = db.QueryRow("select MIN(w_seq) from words where  text_id = ? AND ( processed IS NULL OR processed =0) ", t_id).Scan(&min_seq)

	if err != nil {
		// maybe nothing there??
		panic(err)
	}
	return min_seq
}

func dbGetTotalWords(db *sql.DB, t_id int) int {
	var max_seq int
	err := db.QueryRow("select MAX(w_seq) from words where text_id = ?", t_id).Scan(&max_seq)
	if err != nil {
		panic(err)
	}
	return max_seq
}

//override last processed word, make it unprocessed
func dbGoBack(db *sql.DB, t_id int) {
	var max_seq int
	err := db.QueryRow("select MAX(w_seq) from words where  text_id = ? AND  processed =1 ", t_id).Scan(&max_seq)
	if err != nil {
		return
		panic(err)
	}
	_, err3 := db.Exec(`update words
set processed = 0 where w_seq= ? and text_id = ?`, max_seq, t_id)
	if err3 != nil {
		panic(err3)
	}

}

/* get total number of proccesed words in text*/
func dbGetAccomplished(db *sql.DB, text_id int) int {
	var count_seq int
	err := db.QueryRow("select count(w_seq) from words where  text_id = ? AND (processed =1  OR processed = 2)", text_id).Scan(&count_seq)
	if err != nil {
		panic(err)
	}
	return count_seq
}

/* check user credentials, return true if exists */
func dbCheckUser(db *sql.DB, u_id int, username string) bool {
	var db_id int
	err := db.QueryRow("select u_id from users where u_id = ? AND username = ?", u_id, username).Scan(&db_id)
	if err != nil {
		if err == sql.ErrNoRows {
			return false
		}
		panic(err)
		return false
	}
	return true
}

/* bring a currently worked on text id (not yet finished).
If not available, match with new one. */
func dbWorksOn(db *sql.DB, uid int) int {
	var id int
	var err error
	err = db.QueryRow(`select e.text_id from workson as e
    join texts as d on d.t_id = e.text_id
    where e.user_id = ? AND d.processed = 0`,
		uid).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			// nothing found ?? match new one
			fmt.Println("no text found for user .. matching new one")
			err = db.QueryRow(`select t_id from texts where 
					processed =0
					EXCEPT
					select text_id from workson`).Scan(&id)
			if err != nil {
				if err == sql.ErrNoRows {
					// no more text
					return -1
				}
				panic(err)
			} else {
				_, err = db.Exec(`INSERT INTO workson (user_id, text_id) VALUES (?,?)`,
					uid, id)
				if err != nil {
					panic(err)
				}
				return id
			}
		}
		panic(err)
	}
	return id
}

func dbUpdateBulk(db *sql.DB, t_id int) {
	_, err := db.Exec("UPDATE texts SET processed = 1 WHERE t_id = ?", t_id)
	if err != nil {
		panic(err)
	}
}

func randomToken(length int) string {
	/*
	   https://www.socketloop.com/tutorials/golang-how-to-generate-random-string
	*/
	size := length
	rb := make([]byte, size)
	_, err := rand.Read(rb)

	if err != nil {
		fmt.Println(err)
	}
	rs := base64.URLEncoding.EncodeToString(rb)
	return rs
}
