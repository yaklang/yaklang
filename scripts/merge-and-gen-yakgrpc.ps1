try {
    $protoc = Get-Command protoc
#    Write-Host "Found protoc version $($protoc.Version)" -ForegroundColor Green
} catch {
    Write-Host "protoc is not installed. Please download and install it." -ForegroundColor Red
    Write-Host "https://github.com/protocolbuffers/protobuf/releases" -ForegroundColor Green
    exit
}

try {
    $protocGenGo = Get-Command protoc-gen-go
#    Write-Host "Found protoc-gen-go version $($protocGenGo.Version)" -ForegroundColor Green
} catch {
    Write-Host "protoc-gen-go is not installed. Please download and install it." -ForegroundColor Red
    Write-Host "go install google.golang.org/protobuf/cmd/protoc-gen-go@latest" -ForegroundColor Green
    exit
}

try {
    $protocGenGoGrpc = Get-Command protoc-gen-go-grpc
#    Write-Host "Found protoc-gen-go-grpc version $($protocGenGoGrpc.Version)" -ForegroundColor Green
} catch {
    Write-Host "protoc-gen-go-grpc is not installed. Please download and install it." -ForegroundColor Red
    Write-Host "go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest" -ForegroundColor Green
    exit
}

$currentDir = (Get-Location).Path

Write-Output "Current directory is: $currentDir"

# Check if the current directory ends with \scripts
if ($currentDir -like "*\scripts") {
    cd ..
}

# Run the main.go file , merge protos
go run common/mergeproto/cmd/main.go

# Check if the yakgrpc.proto file exists
if (Test-Path ./common/yakgrpc/yakgrpc.proto) {
    Write-Host "yakgrpc.proto file exists. Proceeding to protoc command..." -ForegroundColor Green

    # Store your command and its arguments in variables
    $protocCommand = "protoc"
    $protocArgs = "--go-grpc_out=common/yakgrpc/ypb", "--go_out=common/yakgrpc/ypb", "--proto_path=common/yakgrpc/", "yakgrpc.protox"

    # Print the command with a newline
    Write-Host "`nExecuting command:`n" -ForegroundColor Yellow
    Write-Host "$protocCommand $($protocArgs -join ' ')`n" -ForegroundColor Green

    # Try to run your protoc command here
    try {
        & $protocCommand $protocArgs
        if ($LASTEXITCODE -ne 0) {
            throw "Command exited with status $LASTEXITCODE"
        }
        Write-Host "`nCommand executed successfully." -ForegroundColor Green
    }
    catch {
        Write-Host "`nError executing command: $_" -ForegroundColor Red
    }
} else {
    Write-Host "yakgrpc.proto file does not exist. Exiting..."  -ForegroundColor Red
}

cd $currentDir
