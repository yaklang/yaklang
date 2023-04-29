package autocomplition

import (
	"github.com/k0kubun/pp"
	"yaklang/common/log"
	"testing"
)

func TestGetBashHistoryTreeRawLines(t *testing.T) {
	baseRaw := `
brew
cd /usr/local/
ls
mkdir homebrew && curl -L https://github.com/Homebrew/brew/tarball/master | tar xz --strip 1 -C homebrew
brew
cd
mkdir homebrew && curl -L https://github.com/Homebrew/brew/tarball/master | tar xz --strip 1 -C homebrew
ls
rm -rf homebrew/
ls
/usr/bin/ruby -e "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/master/install)"
brew install zsh
vim /etc/shells
sudo vim /etc/shells
sudo chsh -s /usr/local/bin/zsh
chsh -s /usr/local/bin/zsh
sudo chsh -s /bin/bash
exit
cat /etc/shells
exit
ls
cd sqlidetector/
ls
zsh
ls
python -m unittest
python -m unittest
python -m unittest test_errorbase.py
..
cd ..
ks
 brew install ctags
ls
python -m unittest
python -m unittest sqlidetector/test_errorbase.py
zsh
zsh
exit
exit
exit
`
	zshRaw := `
: 1568184418:0;git push origin feature/implement-browser-crawler  -f
: 1568184750:0;python2
: 1568184759:0;pyenv local 2.7.13
: 1568184762:0;python2 core/assassin/dev/license/license.py go.mod vendor > core/assassin/dev/license/license.txt
: 1568184957:0;git diff
: 1568184987:0;go mod vendor
: 1568184992:0;python2 core/assassin/dev/license/license.py go.mod vendor > core/assassin/dev/license/license.txt
: 1568184995:0;git add .
: 1568185007:0;git commit -m "add license for chromedp"
: 1568185014:0;lv
: 1568185016:0;vim .gitignore
: 1568185025:0;git diff
: 1568185034:0;git rm --cache .python-version
: 1568185039:0;git status
: 1568185044:0;git add .gitignore
: 1568185054:0;cat .python-version
: 1568185056:0;git add .
: 1568185076:0;git commit -m "remove .python-version"
: 1568185087:0;git push origin feature/implement-browser-crawler
: 1568185291:0;git checkout master
: 1568185437:0;git pull origin master
: 1568185475:0;.
: 1568185560:0;cd Project/
: 1568185560:0;ls
: 1568185564:0;cd
: 1568185569:0;cat .bash_history
: 1568185621:0;cat .zsh_history
: 1568187382:0;gunkit
: 1568187440:0;gunkit --log-level debug fp --host 10.3.0.1/24 --port 22,8080,80,443,10022
: 1568187458:0;gunkit --log-level trace fp --host 10.3.0.1/24 --port 22,8080,80,443,10022
: 1568187470:0;gunkit --log-level trace fp --host 10.3.0.1/24 --port 22,8080,80,443,10022 -a
: 1568187487:0;gunkit --log-level trace fp --host 10.3.0.1/24 --port 22,8080,80,443,10022 timeout 5s
: 1568187537:0;cd Project
: 1568187537:0;ls
: 1568187556:0;cd gunkit
: 1568187557:0;ls
: 1568187596:0;gunkit
: 1568187617:0;R31_SERVER=r31.villanch.top:10235 go run r31c/cmd/r31cli.go
: 1568187625:0;cd Project
: 1568187625:0;ls
: 1568187627:0;cd r31
: 1568187627:0;ls
: 1568187629:0;R31_SERVER=r31.villanch.top:10235 go run r31c/cmd/r31cli.go
: 1568187705:0;cd Project
: 1568187707:0;ls
: 1568187709:0;cd r31
: 1568187710:0;open .
: 1568188456:0;gunkit --log-level trace fp --host 10.3.0.1/24 --port 22,8080,80 --timeout 5s
: 1568189019:0;bash -c "gunkit"
: 1568189041:0;bash -c "htop"
: 1568189048:0;gunkt
: 1568189051:0;gunkit
: 1568189311:0;git diff
: 1568189315:0;git reset --hard
: 1568189316:0;git diff
: 1568189981:0;tail -n 200 ~/.bash_history
: 1568190172:0;tail -n 200 ~/.zsh_history
`
	_ = baseRaw
	_ = zshRaw

	raw := `
mkdir homebrew && curl -L https://github.com/Homebrew/brew/tarball/master | tar xz --strip 1 -C homebrew
`
	forest := GetDefaultSystemHistoryAutoComplitionForest(getBashHistoryRawLines([]byte(raw))...)

	log.Info(pp.Sprintln(len(forest.trees)))

	suggest := forest.GetSuggest("ssh")
	log.Info(pp.Sprintln(suggest))
}
