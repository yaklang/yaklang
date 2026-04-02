<div style="margin:10px;">
<h1>Elasticsearch in dotCMS</h1>

<br>&nbsp;<br>
<h3>Content query: images under the image folder, with limit and offset</h3>
Note the \\ to escape the /
<pre><code>
{
    "query" :
    {
        "query_string" :
        {
            "query" : "+path:\\/images* +metadata.contenttype:image\\/png"
        }
    },
    "size":10,
    "from":5
}
</code></pre>

<br>&nbsp;<br>

<h2>Viewtool<a name="#esPortletViewtool"></a></h2>
<hr>
<p>This is an example of how you can use the $estool to in Velocity to pull content from dotCMS on a velocity page.</p>
<h3>Pull content where title contains "gas"</h3>
<pre><code>#set($results = $estool.search('{
    "query": {
        "bool": {
            "must": {
                "term": {
                    "title": "gas"
                }
            }
        }
    }
}'
))

#foreach($con in $results)
  $con.title&lt;br&gt;
#end
&lt;hr&gt;
$results.response&lt;br&gt;

</code></pre>
</div>
