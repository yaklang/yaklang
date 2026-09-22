// Offline edge cases encoded and decoded by Apache Kafka 3.9.1 generated codecs.
import java.nio.*;
import java.nio.file.*;
import java.util.*;
import com.fasterxml.jackson.databind.*;
import com.fasterxml.jackson.databind.node.*;
import org.apache.kafka.common.message.*;
import org.apache.kafka.common.protocol.*;
import org.apache.kafka.common.protocol.types.RawTaggedField;
import org.apache.kafka.common.record.MemoryRecords;

public class KafkaFlexTags {
 static ObjectMapper json=new ObjectMapper();
 static ArrayNode rows=json.createArrayNode();
 static void sample(String name,ApiMessage message,short version,boolean request)throws Exception{
  byte[] body=MessageUtil.byteBufferToArray(MessageUtil.toByteBuffer(message,version));
  ApiMessage decoded=message.getClass().getConstructor().newInstance();
  ByteBuffer b=ByteBuffer.wrap(body);decoded.read(new ByteBufferAccessor(b),version);
  if(b.hasRemaining()||!message.equals(decoded))throw new IllegalStateException("codec mismatch "+name);
  ObjectNode row=rows.addObject();row.put("name",name);row.put("api",message.apiKey());row.put("version",version);row.put("request",request);row.put("body_hex",HexFormat.of().formatHex(body));
  row.set("expected",(JsonNode)Class.forName(message.getClass().getName()+"JsonConverter").getMethod("write",message.getClass(),short.class).invoke(null,decoded,version));
 }
 public static void main(String[] args)throws Exception{
  var epoch=new FetchResponseData.EpochEndOffset().setEpoch(7).setEndOffset(123);
  epoch.unknownTaggedFields().add(new RawTaggedField(42,new byte[]{4,2}));
  var partition=new FetchResponseData.PartitionData().setPartitionIndex(2).setHighWatermark(200).setLastStableOffset(190).setLogStartOffset(3)
   .setDivergingEpoch(epoch).setCurrentLeader(new FetchResponseData.LeaderIdAndEpoch().setLeaderId(5).setLeaderEpoch(9))
   .setSnapshotId(new FetchResponseData.SnapshotId().setEndOffset(120).setEpoch(6)).setRecords(null).setAbortedTransactions(null);
  partition.unknownTaggedFields().add(new RawTaggedField(55,new byte[]{5}));
  var fetch=new FetchResponseData().setResponses(List.of(new FetchResponseData.FetchableTopicResponse().setTopic("tags").setPartitions(List.of(partition))));
  sample("fetch-known-tags-null",fetch,(short)12,false);
  partition.setRecords(MemoryRecords.EMPTY).setAbortedTransactions(List.of());
  sample("fetch-known-tags-empty",fetch,(short)12,false);
  sample("fetch-cluster-tag",new FetchRequestData().setClusterId("cluster-tag").setRackId("rack-a"),(short)12,true);
  var api=new ApiVersionsResponseData().setZkMigrationReady(true);
  sample("api-migration-tag",api,(short)3,false);
  var errors=new ProduceResponseData.PartitionProduceResponse().setIndex(2).setErrorCode((short)87).setErrorMessage("rejected")
   .setRecordErrors(List.of(new ProduceResponseData.BatchIndexAndErrorMessage().setBatchIndex(3).setBatchIndexErrorMessage(null),new ProduceResponseData.BatchIndexAndErrorMessage().setBatchIndex(4).setBatchIndexErrorMessage("")));
  var topics=new ProduceResponseData.TopicProduceResponseCollection();topics.add(new ProduceResponseData.TopicProduceResponse().setName("tags").setPartitionResponses(List.of(errors)));
  sample("produce-record-errors",new ProduceResponseData().setResponses(topics),(short)9,false);
  Files.writeString(Path.of(args[0]),json.writerWithDefaultPrettyPrinter().writeValueAsString(rows)+"\n");
 }
}
