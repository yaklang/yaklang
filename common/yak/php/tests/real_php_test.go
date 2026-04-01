package tests

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/coreplugin"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

const parsedownMiniPath = "/var/www/html/parsedown_mini.php"

const parsedownMiniCode = `<?php
class ParsedownMini
{
    protected $DefinitionData = array();
    protected $BlockTypes = array(
        '#' => array('Header'),
        '*' => array('List'),
    );
    protected $unmarkedBlockTypes = array();
    protected $InlineTypes = array(
        '[' => array('Link'),
    );
    protected $inlineMarkerList = '[]';

    protected function linesElements(array $lines)
    {
        $CurrentBlock = null;
        foreach ($lines as $line)
        {
            $Line = array('body' => $line, 'text' => $line, 'indent' => 0);
            if (isset($CurrentBlock['continuable']))
            {
                $methodName = 'block' . $CurrentBlock['type'] . 'Continue';
                $Block = $this->$methodName($Line, $CurrentBlock);
                if (isset($Block))
                {
                    $CurrentBlock = $Block;
                    continue;
                }
                if ($this->isBlockCompletable($CurrentBlock['type']))
                {
                    $methodName = 'block' . $CurrentBlock['type'] . 'Complete';
                    $CurrentBlock = $this->$methodName($CurrentBlock);
                }
            }
            $marker = $Line['text'][0];
            $blockTypes = $this->unmarkedBlockTypes;
            if (isset($this->BlockTypes[$marker]))
            {
                foreach ($this->BlockTypes[$marker] as $blockType)
                {
                    $blockTypes []= $blockType;
                }
            }
            foreach ($blockTypes as $blockType)
            {
                $Block = $this->{"block$blockType"}($Line, $CurrentBlock);
                if (isset($Block))
                {
                    $CurrentBlock = $Block;
                    break;
                }
            }
        }

        return $CurrentBlock;
    }

    protected function lineElements($text, $nonNestables = array())
    {
        while ($excerpt = strpbrk($text, $this->inlineMarkerList))
        {
            $marker = $excerpt[0];
            $Excerpt = array('text' => $excerpt, 'context' => $text);
            foreach ($this->InlineTypes[$marker] as $inlineType)
            {
                if (isset($nonNestables[$inlineType]))
                {
                    continue;
                }
                $Inline = $this->{"inline$inlineType"}($Excerpt);
                if (!isset($Inline))
                {
                    continue;
                }
                break;
            }
            $text = substr($text, 1);
        }
        return array();
    }

    protected function handle(array $Element)
    {
        if (is_string($Element['handler']))
        {
            $function = $Element['handler'];
            $argument = $Element['text'];
            $destination = 'rawHtml';
        }
        else
        {
            $function = $Element['handler']['function'];
            $argument = $Element['handler']['argument'];
            $destination = $Element['handler']['destination'];
        }

        $Output = $this->$function($argument);
        $Element[$destination] = $Output;

        return $Element;
    }

    protected function storeReference($id, $Data)
    {
        $this->DefinitionData['Reference'][$id] = $Data;
    }

    protected function resolveReference($definition)
    {
        if (!isset($this->DefinitionData['Reference'][$definition]))
        {
            return null;
        }

        $Definition = $this->DefinitionData['Reference'][$definition];
        return $Definition;
    }

    protected function blockHeader($Line, $CurrentBlock = null)
    {
        return array('type' => 'Header', 'continuable' => true);
    }

    protected function blockHeaderContinue($Line, array $CurrentBlock)
    {
        return null;
    }

    protected function blockHeaderComplete(array $CurrentBlock)
    {
        return $CurrentBlock;
    }

    protected function blockList($Line, $CurrentBlock = null)
    {
        return array(
            'type' => 'List',
            'continuable' => true,
            'li' => array(
                'handler' => array(
                    'argument' => array(),
                ),
            ),
            'element' => array(
                'elements' => array(),
            ),
        );
    }

    protected function blockListContinue($Line, array $CurrentBlock)
    {
        $CurrentBlock['li']['handler']['argument'] []= $Line['text'];
        return $CurrentBlock;
    }

    protected function blockListComplete(array $CurrentBlock)
    {
        return $CurrentBlock;
    }

    protected function isBlockCompletable($type)
    {
        return true;
    }

    protected function inlineLink($Excerpt)
    {
        return array(
            'ok' => 1,
        );
    }

    public function run()
    {
        $this->storeReference('demo', array('title' => 'ok'));
        $this->resolveReference('demo');
        $this->linesElements(array('# Heading', '* item'));
        $this->handle(array(
            'handler' => array(
                'function' => 'lineElements',
                'argument' => '[handler]',
                'destination' => 'elements',
            ),
            'text' => '[fallback]',
        ));
    }
}

$parser = new ParsedownMini();
$parser->run();
`

func useTempPHPRealSSADB(t *testing.T) *gorm.DB {
	t.Helper()

	originDB := consts.GetGormSSAProjectDataBase()
	tempDB, err := consts.GetTempSSADataBase()
	require.NoError(t, err)

	consts.SetGormSSAProjectDatabase(tempDB)
	require.NoError(t, coreplugin.ForceSyncCorePlugin())

	t.Cleanup(func() {
		_ = tempDB.Close()
		consts.SetGormSSAProjectDatabase(originDB)
	})

	return tempDB
}

func getVirtualPHPRuns(t *testing.T) int {
	t.Helper()

	const defaultRuns = 3
	raw := strings.TrimSpace(os.Getenv("YAK_PHP_VIRTUAL_RUNS"))
	if raw == "" {
		return defaultRuns
	}

	runs, err := strconv.Atoi(raw)
	require.NoError(t, err)
	require.GreaterOrEqual(t, runs, 2)
	return runs
}

func countProgramIrCodesByName(t *testing.T, programName string) int64 {
	t.Helper()

	var count int64
	require.NoError(t, ssadb.GetDB().Model(&ssadb.IrCode{}).Where("program_name = ?", programName).Count(&count).Error)
	return count
}

func countProgramIrCodesBySource(t *testing.T, programName, folderPath, fileName string) int64 {
	t.Helper()

	var count int64
	require.NoError(t,
		ssadb.GetDB().
			Model(&ssadb.IrCode{}).
			Joins("join ir_sources on ir_codes.program_name = ir_sources.program_name and ir_codes.source_code_hash = ir_sources.source_code_hash").
			Where("ir_codes.program_name = ?", programName).
			Where("ir_sources.file_name = ?", fileName).
			Where("(ir_sources.folder_path = ? or ir_sources.folder_path like ?)", folderPath, "%"+folderPath).
			Count(&count).Error,
	)
	return count
}

func countProgramOpcodeBySource(t *testing.T, programName, folderPath, fileName, opcode string) int64 {
	t.Helper()

	var count int64
	require.NoError(t,
		ssadb.GetDB().
			Model(&ssadb.IrCode{}).
			Joins("join ir_sources on ir_codes.program_name = ir_sources.program_name and ir_codes.source_code_hash = ir_sources.source_code_hash").
			Where("ir_codes.program_name = ?", programName).
			Where("ir_sources.file_name = ?", fileName).
			Where("(ir_sources.folder_path = ? or ir_sources.folder_path like ?)", folderPath, "%"+folderPath).
			Where("ir_codes.opcode_name = ?", opcode).
			Count(&count).Error,
	)
	return count
}

func buildParsedownMiniFS() *filesys.VirtualFS {
	vf := filesys.NewVirtualFs()
	vf.AddFile(parsedownMiniPath, parsedownMiniCode)
	return vf
}

func TestRealPHP_VirtualFSCompileStability(t *testing.T) {
	if os.Getenv("YAK_PHP_RUN_VIRTUAL_STABILITY") == "" {
		t.Skip("set YAK_PHP_RUN_VIRTUAL_STABILITY=1 to run local PHP virtual-fs compile stability test")
	}

	useTempPHPRealSSADB(t)

	vf := buildParsedownMiniFS()
	folderPath, fileName := path.Split(parsedownMiniPath)
	runs := getVirtualPHPRuns(t)

	var baselineTotal int64
	var baselineFileCount int64
	var baselineParameterMember int64
	var baselinePhi int64

	for run := 1; run <= runs; run++ {
		programName := fmt.Sprintf("%s-run-%d", strings.ToLower(t.Name()), run)
		progs, err := ssaapi.ParseProjectWithFS(
			vf,
			ssaapi.WithLanguage(ssaconfig.PHP),
			ssaapi.WithProgramName(programName),
		)
		require.NoError(t, err)
		require.Len(t, progs, 1)
		require.Equal(t, programName, progs[0].GetProgramName())

		total := countProgramIrCodesByName(t, programName)
		fileCount := countProgramIrCodesBySource(t, programName, folderPath, fileName)
		parameterMember := countProgramOpcodeBySource(t, programName, folderPath, fileName, "ParameterMember")
		phi := countProgramOpcodeBySource(t, programName, folderPath, fileName, "Phi")

		t.Logf("run=%d program=%s total=%d file=%d parameter_member=%d phi=%d", run, programName, total, fileCount, parameterMember, phi)

		defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

		if run == 1 {
			baselineTotal = total
			baselineFileCount = fileCount
			baselineParameterMember = parameterMember
			baselinePhi = phi
			continue
		}

		require.Equal(t, baselineTotal, total, "total ir_codes changed across runs")
		require.Equal(t, baselineFileCount, fileCount, "single-file ir_codes changed across runs")
		require.Equal(t, baselineParameterMember, parameterMember, "ParameterMember count changed across runs")
		require.Equal(t, baselinePhi, phi, "Phi count changed across runs")
	}
}
