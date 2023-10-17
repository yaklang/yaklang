// Package simulator
// @Author bcy2007  2023/8/17 16:21
package simulator

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const TestLoginHTMLA = `<html>
  <script id="allow-copy_script">
    (function agent() {
      let unlock = false;
      document.addEventListener("allow_copy", (event) => {
        unlock = event.detail.unlock;
      });

      const copyEvents = [
        "copy",
        "cut",
        "contextmenu",
        "selectstart",
        "mousedown",
        "mouseup",
        "mousemove",
        "keydown",
        "keypress",
        "keyup",
      ];
      const rejectOtherHandlers = (e) => {
        if (unlock) {
          e.stopPropagation();
          if (e.stopImmediatePropagation) e.stopImmediatePropagation();
        }
      };
      copyEvents.forEach((evt) => {
        document.documentElement.addEventListener(evt, rejectOtherHandlers, {
          capture: true,
        });
      });
    })();
  </script>
  <head>
    <meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />

    <!--<link rel="stylesheet" type="text/css" href="https://fonts.googleapis.com/css?family=Architects+Daughter">-->
    <link
      rel="stylesheet"
      type="text/css"
      href="stylesheets/stylesheet.css"
      media="screen"
    />
    <link rel="shortcut icon" href="images/favicon.ico" type="image/x-icon" />

    <!--<script src="//html5shiv.googlecode.com/svn/trunk/html5.js"></script>-->
    <script src="js/html5.js"></script>

    <title>bWAPP - Login</title>
  </head>

  <body>
    <header>
      <h1>bWAPP</h1>
      <h2>an extremely buggy web app !</h2>
    </header>

    <div id="menu">
      <table>
        <tbody>
          <tr>
            <td><font color="#ffb717">Login</font></td>
            <td><a href="user_new.php">New User</a></td>
            <td><a href="info.php">Info</a></td>
            <td><a href="training.php">Talks &amp; Training</a></td>
            <td>
              <a href="http://itsecgames.blogspot.com" target="_blank">Blog</a>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div id="main">
      <h1>Login</h1>

      <p>Enter your credentials <i>(bee/bug)</i>.</p>

      <form action="/bwapp/login.php" method="POST">
        <p>
          <label for="login">Login:</label><br />
          <input type="text" id="user" name="user" size="20" autocomplete="off" />
        </p>

        <p>
          <label for="password">Password:</label><br />
          <input type="password" id="password" name="password" size="20" autocomplete="off" />
        </p>

        <p>
          <label for="security_level">Set the security level:</label><br />

          <select name="security_level">
            <option value="0">low</option>
            <option value="1">medium</option>
            <option value="2">high</option>
          </select>
        </p>

        <button type="submit" name="form" value="submit">Login</button>
      </form>

      <br />
    </div>

    <div id="sponsor_2">
      <table>
        <tbody>
          <tr>
            <td width="103" align="center">
              <a href="https://www.owasp.org" target="_blank">
				<img src="./images/owasp.png" />
			  </a>
            </td>
            <td width="102" align="center">
              <a href="https://www.owasp.org/index.php/OWASP_Zed_Attack_Proxy_Project" target="_blank">
				<img src="./images/zap.png" />
			  </a>
            </td>
            <td width="110" align="center">
              <a href="https://www.netsparker.com/?utm_source=bwappapp&amp;utm_medium=banner&amp;utm_campaign=bwapp" target="_blank">
				<img src="./images/netsparker.png" />
			  </a>
            </td>
            <td width="152" align="center">
              <a href="http://www.missingkids.com" target="_blank">
				<img src="./images/mk.png" />
			  </a>
            </td>
          </tr>
        </tbody>
      </table>

      <br />

      <table>
        <tbody>
          <tr>
            <td width="288" align="right">
              <a href="http://www.mmebvba.com" target="_blank">
				<img src="./images/mme.png" />
			  </a>
            </td>
            <td width="190" align="right">
              <a href="https://www.netsparker.com/?utm_source=bwappapp&amp;utm_medium=banner&amp;utm_campaign=bwapp" target="_blank">
				<img src="./images/netsparker.gif" />
			  </a>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div id="side">
      <a href="http://twitter.com/MME_IT" target="blank_" class="button">
		<img src="./images/twitter.png" />
	  </a>
      <a href="http://be.linkedin.com/in/malikmesellem" target="blank_" class="button">
		<img src="./images/linkedin.png" />
	  </a>
      <a href="http://www.facebook.com/pages/MME-IT-Audits-Security/104153019664877" target="blank_" class="button">
		<img src="./images/facebook.png" />
	  </a>
      <a href="http://itsecgames.blogspot.com" target="blank_" class="button">
		<img src="./images/blogger.png" />
	  </a>
    </div>

    <div id="disclaimer">
      <p>
        bWAPP is licensed under
        <a rel="license" href="http://creativecommons.org/licenses/by-nc-nd/4.0/" target="_blank">
		  <img style="vertical-align: middle" src="./images/cc.png" />
		</a>
        © 2014 MME BVBA / Follow
        <a href="http://twitter.com/MME_IT" target="_blank">@MME_IT</a> on
        Twitter and ask for our cheat sheet, containing all solutions! / Need an
        exclusive <a href="http://www.mmebvba.com" target="_blank">training</a>?
      </p>
    </div>

    <div id="bee">
      <img src="./images/bee_1.png" />
    </div>
  </body>
</html>`

func TestElementFind(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html := TestLoginHTMLA
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()
	starter := CreateNewStarter()
	err := starter.Start()
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		_ = starter.Close()
	}()
	page, err := starter.CreatePage()
	if err != nil {
		t.Error(err)
		return
	}
	err = page.Navigate(server.URL)
	if err != nil {
		t.Error(err)
		return
	}
	err = page.WaitLoad()
	if err != nil {
		t.Error(err)
		return
	}
	searchInfo := map[string]map[string][]string{
		"input": {
			"type": {
				"text",
				"password",
				"number",
				"tel",
			},
		},
	}
	elements, err := customizedGetElement(page, searchInfo)
	if err != nil {
		t.Error(err)
		return
	}
	//t.Log(ElementsToSelectors(elements...))
	//t.Log(ElementsToIds(elements...))
	tags := []string{"Username", "Password", "Captcha"}
	result, err := CalculateRelevanceMatrix(elements, tags)
	if err != nil {
		t.Error(err)
	}
	t.Log(result)
}
