// Run against the isolated Kafka 3.9.1 fixture broker while capturing TCP 19092.
// Kafka's generated Java message codecs are the independent wire/JSON oracle.
import java.net.*;
import java.io.*;
import java.nio.*;
import java.nio.file.*;
import java.util.*;
import com.fasterxml.jackson.databind.*;
import com.fasterxml.jackson.databind.node.*;
import org.apache.kafka.common.message.*;
import org.apache.kafka.common.protocol.*;
import org.apache.kafka.common.protocol.types.RawTaggedField;
import org.apache.kafka.common.record.*;
import org.apache.kafka.common.compress.Compression;

public class KafkaFlex {
 static ObjectMapper json=new ObjectMapper();
 static ArrayNode samples=json.createArrayNode();
 static Socket socket;
 static int correlation=100;
 static JsonNode asJson(Message m,short version)throws Exception{
  return (JsonNode)Class.forName(m.getClass().getName()+"JsonConverter").getMethod("write",m.getClass(),short.class).invoke(null,m,version);
 }
 static byte[] bytes(Message m,short v){return MessageUtil.byteBufferToArray(MessageUtil.toByteBuffer(m,v));}
 static void exchange(ApiMessage request,short version,boolean reply)throws Exception{
  ApiKeys key=ApiKeys.forId(request.apiKey());int corr=++correlation;
  RequestHeaderData h=new RequestHeaderData().setRequestApiKey(request.apiKey()).setRequestApiVersion(version).setCorrelationId(corr).setClientId("zip-flex-java");
  h.unknownTaggedFields().add(new RawTaggedField(70,new byte[]{9,8}));
  request.unknownTaggedFields().add(new RawTaggedField(99,new byte[]{7,6,5}));
  byte[] header=bytes(h,key.requestHeaderVersion(version)),body=bytes(request,version);
  ByteBuffer wire=ByteBuffer.allocate(4+header.length+body.length).putInt(header.length+body.length).put(header).put(body);
  socket.getOutputStream().write(wire.array());socket.getOutputStream().flush();
  ObjectNode row=samples.addObject();row.put("api",request.apiKey());row.put("version",version);row.put("correlation",corr);row.put("request_header_version",key.requestHeaderVersion(version));row.put("response_header_version",key.responseHeaderVersion(version));row.put("request_hex",HexFormat.of().formatHex(wire.array()));row.set("request",asJson(request,version));
  if(!reply){row.putNull("response_hex");return;}
  DataInputStream in=new DataInputStream(socket.getInputStream());int n=in.readInt();if(n<4||n>1048576)throw new IOException("response budget");byte[] rsp=in.readNBytes(n);if(rsp.length!=n)throw new EOFException();
  ByteBuffer b=ByteBuffer.wrap(rsp);ResponseHeaderData rh=new ResponseHeaderData(new ByteBufferAccessor(b),key.responseHeaderVersion(version));if(rh.correlationId()!=corr)throw new IOException("correlation mismatch");
  ApiMessage response;
  switch(request.apiKey()){case 18:response=new ApiVersionsResponseData(new ByteBufferAccessor(b),version);break;case 3:response=new MetadataResponseData(new ByteBufferAccessor(b),version);break;case 0:response=new ProduceResponseData(new ByteBufferAccessor(b),version);break;case 1:response=new FetchResponseData(new ByteBufferAccessor(b),version);break;default:throw new IOException();}
  if(b.hasRemaining())throw new IOException("oracle left trailing bytes");
  row.put("response_hex",HexFormat.of().formatHex(ByteBuffer.allocate(4+n).putInt(n).put(rsp).array()));row.set("response",asJson(response,version));
  if(response instanceof ProduceResponseData){for(var t:((ProduceResponseData)response).responses())for(var p:t.partitionResponses())if(p.errorCode()!=0)throw new IOException("produce error "+p);}
 }
 static ProduceRequestData produce(boolean gzip,short acks){
  var partitions=new ArrayList<ProduceRequestData.PartitionProduceData>();
  for(int i=0;i<2;i++){
   var records=MemoryRecords.withRecords(gzip?Compression.gzip().build():Compression.NONE,new SimpleRecord(0L,null,("zip-flex-p"+i+"-gzip"+gzip).getBytes(java.nio.charset.StandardCharsets.UTF_8)));
   partitions.add(new ProduceRequestData.PartitionProduceData().setIndex(i).setRecords(records));
  }
  var topics=new ProduceRequestData.TopicProduceDataCollection();topics.add(new ProduceRequestData.TopicProduceData().setName("zip-flex").setPartitionData(partitions));
  return new ProduceRequestData().setTransactionalId(null).setAcks(acks).setTimeoutMs(5000).setTopicData(topics);
 }
 public static void main(String[] args)throws Exception{
  socket=new Socket("127.0.0.1",19092);socket.setSoTimeout(10000);
  try{
   exchange(new ApiVersionsRequestData().setClientSoftwareName("zip-flex-java").setClientSoftwareVersion("3.9.1"),(short)3,true);
   exchange(new MetadataRequestData().setTopics(List.of()).setAllowAutoTopicCreation(false),(short)9,true);
   exchange(new MetadataRequestData().setTopics(null).setAllowAutoTopicCreation(false),(short)9,true);
   exchange(new MetadataRequestData().setTopics(List.of(new MetadataRequestData.MetadataRequestTopic().setName("zip-flex"))).setAllowAutoTopicCreation(false),(short)9,true);
   exchange(produce(false,(short)1),(short)9,true);exchange(produce(true,(short)1),(short)9,true);
   var parts=new ArrayList<FetchRequestData.FetchPartition>();for(int i=0;i<2;i++)parts.add(new FetchRequestData.FetchPartition().setPartition(i).setFetchOffset(0).setLastFetchedEpoch(-1).setLogStartOffset(-1).setCurrentLeaderEpoch(-1).setPartitionMaxBytes(1048576));
   exchange(new FetchRequestData().setReplicaId(-1).setMaxWaitMs(100).setMinBytes(1).setMaxBytes(1048576).setSessionId(0).setSessionEpoch(-1).setRackId("").setTopics(List.of(new FetchRequestData.FetchTopic().setTopic("zip-flex").setPartitions(parts))),(short)12,true);
   exchange(produce(false,(short)0),(short)9,false);
   exchange(new ApiVersionsRequestData().setClientSoftwareName("zip-flex-java").setClientSoftwareVersion("3.9.1"),(short)3,true);
  }finally{socket.close();}
  Files.writeString(Path.of(args[0]),json.writerWithDefaultPrettyPrinter().writeValueAsString(samples)+"\n");System.out.println("Official Java codec exchanges: "+samples.size());
 }
}
