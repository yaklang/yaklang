<?php
/*
 * Created on 2016-5-19
 *
 * To change the template for this generated file go to
 * Window - Preferences - PHPeclipse - PHP - Code Templates
 */
namespace PHPEMS;

class action extends app
{
	public function display()
	{
        $this->wechat = \PHPEMS\ginkgo::make('wechat');
	    $action = $this->ev->url(3);
		if(!method_exists($this,$action))
		$action = "index";
		$this->$action();
		exit;
	}

	public function index()
	{
		$rev = $this->wechat->getRev();
		$type = strtolower($rev->getRevType());
		switch($type)
		{
			case wechat::MSGTYPE_TEXT:
			case wechat::MSGTYPE_VOICE:
			$this->item = ginkgo::make('item','weixin');
			$content = $rev->getRevContent();
			$content = str_replace(array("。","."),"",$content);
			$item = $this->item->getItemByCode($content);
			$info = array();
			if($item)
			{
				$info[] = array(
					'Title' => $item['itemtitle'],
					'Description' => $item['itemcode'],
					'Url' => WP.'index.php?weixin-phone-index-items&itemids='.$item['itemid']
				);
			}
			else
			{
				exit($this->wechat->text("未搜索到商品")->reply());
			}
			break;

			case wechat::MSGTYPE_IMAGE:
			$content = $rev->getRevContent();
			$picurl = $rev->getRevPic();
			$image = base64_encode(file_get_contents($picurl));
			$baidu = \PHPEMS\ginkgo::make('baidu');
			$res = $baidu->searchItemImg(array('image' => $image,'rn' => 40));
			$items = $res['result'];
			$info = array();
			$goods = array();
			foreach($items as $key => $item)
			{
				$title = json_decode($item['brief'],true);
				if(!$goods[$title['id']])
				{
					$goods[$title['id']] = $title['id'];
					$info[] = array(
						'Title' => $title['title'],
						'Description' => '相似度'.$item['score'],
						'PicUrl' => $picurl,
						'Url' => $picurl
					);
				}
			}
			$info[0]['Url'] = WP.'index.php?weixin-phone-index-items&itemids='.implode($goods);
			break;

			default:
			$this->wechat->text("信息已接收")->reply();
			return false;
		}
		$this->wechat->news($info)->reply();
	}
}


?>
