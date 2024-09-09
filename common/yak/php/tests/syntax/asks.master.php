<?php
 namespace PHPEMS;
/*
 * Created on 2016-5-19
 *
 * To change the template for this generated file go to
 * Window - Preferences - PHPeclipse - PHP - Code Templates
 */
class action extends app
{
	public function display()
	{
		$action = $this->ev->url(3);
		if(!method_exists($this,$action))
		$action = "index";
		$this->$action();
		exit;
	}

	private function del()
	{
		$askid = $this->ev->get('askid');
		$page = $this->ev->get('page');
		$this->answer->delAsksById($askid);
		\PHPEMS\ginkgomsg(array('url'=>'index.php?exam-master-asks&page='.$page));
	}

	private function delanswer()
	{
		$answerid = $this->ev->get('answerid');
		$answer = $this->answer->getAnswerById($answerid);
		$page = $this->ev->get('page');
		$this->answer->delAnswerById($answerid);
		\PHPEMS\ginkgomsg(array('url'=>'index.php?exam-master-asks-detail&askid='.$answer['answeraskid'].'&page='.$page));
	}

	private function done()
	{
		$page = $this->ev->get('page');
		$ids = $this->ev->get('delids');
		foreach($ids as $key => $id)
		{
			$this->answer->delAsksById($id);
		}
		\PHPEMS\ginkgomsg(array('url'=>'index.php?exam-master-asks&page='.$page));
	}

	private function detail()
	{
		$page = $this->ev->get('page');
		$askid = $this->ev->get('askid');
		$ask = $this->answer->getAskById($askid);
		$question = $this->exam->getQuestionByArgs(array(array("AND","questionid = :questionid",'questionid',$ask['askquestionid'])));
		$answers = $this->answer->getAnswerList($page,20,array(array("AND","answeraskid = :answeraskid",'answeraskid',$ask['askid'])));
		$this->tpl->assign('question',$question);
		$this->tpl->assign('answers',$answers);
		$this->tpl->display('ask_answer');
	}

	private function rely()
	{
		$page = $this->ev->get('page');
		$answerid = $this->ev->get('answerid');
		$args = $this->ev->get('args');
		$args['answertime'] = TIME;
		$args['answerteacher'] = $this->_user['sessionusername'];
		$args['answerteacherid'] = $this->_user['sessionuserid'];
		$id = $this->answer->giveAnswer($answerid,$args);
		\PHPEMS\ginkgomsg(array('url'=>'index.php?exam-master-asks-detail&askid='.$id.'&page='.$page));
	}

	private function index()
	{
		$sargs = $this->ev->get('args');
		$page = $this->ev->get('page');
		$page = $page > 1?$page:1;
		$args = array(array("AND","asks.askquestionid = questions.questionid"));
		if($sargs['asksubjectid'])$args[] = array("AND","asks.asksubjectid = :asksubjectid",'asksubjectid',$sargs['asksubjectid']);
		if($sargs['asklasttime'])$args[] = array("AND","asks.asklasttime >= :asklasttime",'asklasttime',$sargs['asklasttime']);
		if($sargs['askuserid'])$args[] = array("AND","asks.asklastteacherid = :asklastteacherid",'asklastteacherid',$sargs['askuserid']);
		if($sargs['askstatus'])
		{
			if($sargs['askstatus'] == -1)
			$args[] = array("AND","asks.askstatus = '0'");
			else
			$args[] = array("AND","asks.askstatus = '1'");
		}
		$subjects = $this->basic->getSubjectList();
		$asks = $this->answer->getAskList($page,20,$args);
		$this->tpl->assign('args',$sargs);
		$this->tpl->assign('asks',$asks);
		$this->tpl->assign('subjects',$subjects);
		$this->tpl->display('asks');
	}
}


?>
