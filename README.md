# Pairing Service

Go 服务，为 4–12 名选手编排锦标赛的下一轮比赛（瑞士制风格）。

## 规则

- **输入**：每位选手的当前积分（以半分为单位的非负整数）、此前每轮的对手与执先颜色、已有轮空记录。
- **历史校验**（不合法直接拒绝，返回 `INVALID_HISTORY`）：
  - 同一选手在同一轮不得重复出场（也不能同时又比赛又轮空）；
  - 每场比赛必须在对手的记录中镜像出现，轮次一致、颜色相反；
  - 轮次必须为正，对手必须是在册选手，颜色必须合法。
- **硬约束**：
  - 下一轮不得与历史对手重复交手；
  - 人数为奇数时恰有一人轮空；已经轮空过的选手不能再次轮空；
  - 每对选手的执先颜色必须使每位选手**赛后**执先/执后次数差的绝对值不超过 2。
- **优化目标**（字典序，只在完整合法方案之间比较）：
  1. 最小化所有配对的双方积分差绝对值之和；
  2. 最小化所有选手赛后颜色次数差绝对值之和（轮空者颜色差不变，同样计入）；
  3. 按选手 ID 排序后的配对与颜色序列裁决（轮空以标记 `B` 参与排序，`B < F < S`）。
- 不存在完整合法方案时返回 `NO_PAIRING`，绝不交付部分赛程。

算法为完整枚举（规模上限 12 人：完美匹配数 10,395 个，奇数人数再乘至多 12 种轮空），
DFS 中增量累计两级成本并用当前最优积分差剪枝，仅在两级目标打平时构造裁决序列。
测试以独立重写的暴力枚举器在随机用例上对拍目标值与并列顺序。

## HTTP 接口

```
POST /pairings/next-round
Content-Type: application/json
```

请求：

```json
{
  "players": [
    {"id": "alice", "score": 6},
    {"id": "bob",   "score": 5}
  ],
  "games": {
    "alice": [{"round": 1, "opponentId": "bob", "color": "F"}],
    "bob":   [{"round": 1, "opponentId": "alice", "color": "S"}]
  },
  "byes": {
    "bob": [2]
  }
}
```

- `score`：半分制整数（1 = 1 个半分）。
- `color`：`"F"`（执先，也接受 `FIRST`/`W`/`WHITE`）或 `"S"`（执后，也接受 `SECOND`/`B`/`BLACK`）。

响应：

| 场景 | 状态码 | Body |
| --- | --- | --- |
| 成功 | 200 | `{"round":2,"pairs":[{"firstId":"alice","secondId":"bob"}],"byeId":""}` |
| 输入不合法 | 400 | `{"status":"INVALID_HISTORY","error":"..."}` |
| 无完整方案 | 422 | `{"status":"NO_PAIRING"}` |

`pairs` 按配对中较小 ID 排序；奇数人数时 `byeId` 为轮空者。
`round` 为历史中最大轮次 + 1（无任何历史时为 1）。

## 库用法

```go
plan, err := pairing.NextRound(req)
switch {
case errors.Is(err, pairing.ErrNoPairing):  // NO_PAIRING
case pairing.IsInvalidHistory(err):         // 输入不合法
case err != nil:                            // 其他错误
default:
    _ = plan // pairing.Plan
}
```

## 运行与测试

```bash
go run ./cmd/pairingd                               # 默认监听 :8080，可用 PAIRING_ADDR 覆盖
go test ./...                                       # 单元测试 + HTTP 测试
go test -run TestAgainstBruteForceOracle -v         # 1000 组随机对拍
go test -run TestLargeHeadCounts -v                 # 10/11/12 人边界
```
