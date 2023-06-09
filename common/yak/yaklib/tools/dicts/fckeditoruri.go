package dicts

import "github.com/yaklang/yaklang/common/utils"

var fckEditorUri = `/admin/FCKeditor/editor/filemanager/browser/default/connectors/test.html
/admin/FCKeditor/editor/filemanager/upload/test.html
/admin/FCKeditor/editor/filemanager/connectors/test.html
/admin/FCKeditor/editor/filemanager/connectors/uploadtest.html
/FCKeditor/editor/filemanager/browser/default/connectors/test.html
/FCKeditor/editor/filemanager/upload/test.html
/FCKeditor/editor/filemanager/connectors/test.html
/FCKeditor/editor/filemanager/connectors/uploadtest.html
/FCKeditor/_samples/default.html
/FCKeditor/_samples/asp/sample01.asp
/FCKeditor/_samples/asp/sample02.asp
/FCKeditor/_samples/asp/sample03.asp
/FCKeditor/_samples/asp/sample04.asp
/admin/FCKeditor/_samples/default.html
/admin/FCKeditor/_samples/asp/sample01.asp
/admin/FCKeditor/_samples/asp/sample02.asp
/admin/FCKeditor/_samples/asp/sample03.asp
/admin/FCKeditor/_samples/asp/sample04.asp
/FCKeditor/editor/filemanager/browser/default/browser.aspx
/FCKeditor/editor/filemanager/connectors/aspx/connector.aspx
/FCKeditor/editor/filemanager/connectors/aspx/connector.aspx1
/admin/FCKeditor/editor/filemanager/connectors/asp/connector.asp
/admin/FCKeditor/editor/filemanager/upload/test.aspx
/admin/fckeditor/editor/filemanager/connectors/aspx/upload.aspx
/admin/FCKeditor/editor/filemanager/connectors/aspx/connector.aspx
/admin/FCKeditor/editor/filemanager/connectors/aspx/connector.aspx1
/fckeditor/editor/fckeditor.html
/admin/fckeditor/editor/fckeditor.html
/fckeditor/
/admin/fckeditor/`

var FckEditorUris = utils.PrettifyListFromStringSplited(fckEditorUri, "\n")
