; SameDesk installer for Windows. Built by `make windows-installer` with NSIS:
;   makensis -DVERSION=1.2.3 -DARCH=x64 -DEXE=path\to\samedesk.exe -DOUTFILE=out.exe samedesk.nsi
; Installs for the current user only, so it never asks for admin rights.

Unicode true
SetCompressor /SOLID lzma

!define APP "SameDesk"
!define UNINST "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP}"
!define RUNKEY "Software\Microsoft\Windows\CurrentVersion\Run"

Name "${APP}"
OutFile "${OUTFILE}"
RequestExecutionLevel user
InstallDir "$LOCALAPPDATA\Programs\${APP}"
BrandingText "${APP} ${VERSION}"

VIProductVersion "${VERSION}.0"
VIAddVersionKey "ProductName" "${APP}"
VIAddVersionKey "FileDescription" "${APP} installer"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "LegalCopyright" "The SameDesk authors, MIT licence"

!include "MUI2.nsh"
!include "FileFunc.nsh"
!define MUI_ICON "..\icons\samedesk.ico"
!define MUI_UNICON "..\icons\samedesk.ico"
!define MUI_FINISHPAGE_RUN "$INSTDIR\${APP}.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Open ${APP}"
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

; Ask a running copy to quit (it shuts its sync engine down cleanly), then make sure.
!macro StopApp
  nsExec::Exec 'taskkill /IM ${APP}.exe'
  Sleep 2000
  nsExec::Exec 'taskkill /F /IM ${APP}.exe'
  Sleep 500
!macroend

Section
  !insertmacro StopApp
  SetOutPath "$INSTDIR"
  File "/oname=${APP}.exe" "${EXE}"
  File "/oname=LICENSE.txt" "..\..\LICENSE"
  File "/oname=NOTICE.txt" "..\..\NOTICE"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  CreateShortcut "$SMPROGRAMS\${APP}.lnk" "$INSTDIR\${APP}.exe"

  WriteRegStr HKCU "${UNINST}" "DisplayName" "${APP}"
  WriteRegStr HKCU "${UNINST}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINST}" "Publisher" "The SameDesk authors"
  WriteRegStr HKCU "${UNINST}" "URLInfoAbout" "https://github.com/SyedAliMasoodBukhari/Samedesk"
  WriteRegStr HKCU "${UNINST}" "DisplayIcon" "$INSTDIR\${APP}.exe"
  WriteRegStr HKCU "${UNINST}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINST}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "${UNINST}" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
  WriteRegDWORD HKCU "${UNINST}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINST}" "NoRepair" 1
  WriteRegDWORD HKCU "${UNINST}" "EstimatedSize" 30000
SectionEnd

; Updates run this installer silently with /RELAUNCH; start the new version then
; (the finish page, which normally offers to open it, isn't shown when silent).
Function .onInstSuccess
  ${GetParameters} $R0
  ClearErrors
  ${GetOptions} $R0 "/RELAUNCH" $R1
  IfErrors +2
  Exec '"$INSTDIR\${APP}.exe" -open=false -restarted'
FunctionEnd

; Removes the program, its shortcut and its start-at-login entry. Settings and
; the shared folder are left alone: they are the user's.
Section "Uninstall"
  !insertmacro StopApp
  Delete "$INSTDIR\${APP}.exe"
  Delete "$INSTDIR\LICENSE.txt"
  Delete "$INSTDIR\NOTICE.txt"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\${APP}.lnk"
  DeleteRegValue HKCU "${RUNKEY}" "${APP}"
  DeleteRegKey HKCU "${UNINST}"
SectionEnd
