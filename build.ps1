# build golang app for windows, gui application
# usage: ./build.ps1

# kill tgsconvert.exe
taskkill /F /IM tgsconvert.exe

# build
$parent = Split-Path -Parent $MyInvocation.MyCommand.Definition
go build -o "$parent/dist/tgsconvert.exe" .
