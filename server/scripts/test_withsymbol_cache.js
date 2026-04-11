// 最小测试脚本：验证 WithSymbol 缓存机制
function onInit() {
  console.log("=== WithSymbol 缓存机制测试 ===");
  
  // 测试1：同一 key 返回同一实例
  var sym1 = WithSymbol("binance", "BTC/USDT:SPOT");
  sym1.Set("testKey", 123);
  
  var sym2 = WithSymbol("binance", "BTC/USDT:SPOT");
  var value = sym2.Get("testKey");
  
  if (value === 123) {
    console.log("✓ 测试通过：WithSymbol 缓存机制正常，sym1.Set() 写入的值能被 sym2.Get() 读到");
  } else {
    console.error("✗ 测试失败：期望值 123，实际值", value);
  }
  
  // 测试2：不同 key 返回不同实例
  var sym3 = WithSymbol("okx", "ETH/USDT:SPOT");
  var value2 = sym3.Get("testKey");
  
  if (value2 === undefined || value2 === null) {
    console.log("✓ 测试通过：不同 exchange+symbol 的 handle 相互独立");
  } else {
    console.error("✗ 测试失败：不同标的不应共享缓存，实际值", value2);
  }
  
  console.log("=== 测试完成 ===");
}

function onSignal(signal) {
  // 不处理信号
}
