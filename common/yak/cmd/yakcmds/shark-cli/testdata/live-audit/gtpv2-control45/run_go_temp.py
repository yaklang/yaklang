from pathlib import Path
import subprocess
task_temp=Path(__file__).parent/'test-temp-02'
task_temp.mkdir(exist_ok=True)
subprocess.run(['compact.exe','/C','/I','/Q',str(task_temp)],check=True,stdout=subprocess.DEVNULL)
source=Path(r'C:\tmp\yak-shark-TLS-metadata-20261028\run_go_phase.py').read_text('utf8')
source=source.replace("root=Path(r'C:\\tmp\\yak-shark-TLS-metadata-20261028')", "root=Path(r'C:\\tmp\\yak-shark-ESP-SSH-UI-20261028')")
source=source.replace("'./common/bin-parser/rules'", "'./common/bin-parser/rules','./common/bin-parser/parser/stream_parser'")
before='GOTMPDIR=str(temp)'
assert source.count(before)==1
source=source.replace(before,'GOTMPDIR=str(temp),TMP=str(task_temp),TEMP=str(task_temp)')
exec(compile(source,str(Path(__file__)),'exec'),globals())
