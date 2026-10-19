package engine

import "fmt"

// priceDecision 表示一次跟价决策的结果。
type priceDecision struct {
	NewPrice int    // -1 表示保持原价
	Action   string // 跟涨 / 跟降 / 底价保护 / 空字符串
	Reason   string
}

// decidePrice 计算目标售价。
//
// 参数说明：
//   - curPrice        我方当前售价
//   - competitorPrice 不含我方的最低竞品价（0 表示无法确定）
//   - bestPrice       卖家 API 返回的市场最低价（buybox 价，包含我方）
//   - minPrice        防亏底价（0 表示未设置）
//   - maxPrice        封顶价（0 表示未设置）
//   - decreaseStep    跟降步长
//   - increaseStep    跟涨步长
//   - soloConfirmed   是否已确认市场上只有我方一个报价
//
// 背景：卖家 API 的 MPV bestPrice 是"市场最低价"，当自己就是最低价时它恒等于
// 我方售价，因此原有逻辑里的跟涨分支永远不可能命中。要实现"跟随涨价"，必须传入
// 真实的最低竞品价 competitorPrice。
func decidePrice(curPrice, competitorPrice, bestPrice, minPrice, maxPrice, decreaseStep, increaseStep int, soloConfirmed bool) priceDecision {
	if decreaseStep <= 0 {
		decreaseStep = 1
	}
	if increaseStep <= 0 {
		increaseStep = 1
	}

	// 1. 已知最低竞品价：以"比竞品低一个步长"为目标，在保住 Buybox 的同时利润最大化。
	if competitorPrice > 0 {
		switch {
		case competitorPrice > curPrice:
			target := competitorPrice - increaseStep
			if maxPrice > 0 && target > maxPrice {
				target = maxPrice
			}
			if target > curPrice {
				return priceDecision{
					NewPrice: target,
					Action:   "跟涨",
					Reason:   fmt.Sprintf("竞品价更高 R%d，跟涨 R%d 至 R%d", competitorPrice, increaseStep, target),
				}
			}
			return priceDecision{
				NewPrice: -1,
				Reason:   fmt.Sprintf("竞品价 R%d 与当前价 R%d 差距不足提价步长 R%d，保持价格", competitorPrice, curPrice, increaseStep),
			}

		case competitorPrice < curPrice:
			target := competitorPrice - decreaseStep
			if minPrice > 0 && target < minPrice {
				target = minPrice
				if target >= curPrice {
					return priceDecision{
						NewPrice: -1,
						Reason:   fmt.Sprintf("竞品低价 R%d，已触碰底价保护 R%d", competitorPrice, minPrice),
					}
				}
				return priceDecision{
					NewPrice: target,
					Action:   "底价保护",
					Reason:   fmt.Sprintf("竞品低价 R%d，触碰底价保护 R%d", competitorPrice, minPrice),
				}
			}
			return priceDecision{
				NewPrice: target,
				Action:   "跟降",
				Reason:   fmt.Sprintf("竞品低价 R%d，跟降 R%d 至 R%d", competitorPrice, decreaseStep, target),
			}

		default:
			return priceDecision{
				NewPrice: -1,
				Reason:   fmt.Sprintf("价格与竞品持平 R%d，保持价格", curPrice),
			}
		}
	}

	// 2. 无法确定竞品价，但已确认独家在售：按提价步长回调价格赚取更多利润，受封顶价保护。
	if soloConfirmed {
		if maxPrice > 0 && curPrice < maxPrice {
			target := curPrice + increaseStep
			if target > maxPrice {
				target = maxPrice
			}
			return priceDecision{
				NewPrice: target,
				Action:   "跟涨",
				Reason:   fmt.Sprintf("独家在售，回调提价 R%d 至 R%d（封顶 R%d）", increaseStep, target, maxPrice),
			}
		}
		return priceDecision{NewPrice: -1}
	}

	// 3. 存在竞品但拿不到竞品报价（例如多变体商品只返回聚合价）：
	//    保守起见只允许跟降，绝不上调，避免误判导致亏损或丢失 Buybox。
	if bestPrice > 0 && bestPrice < curPrice {
		target := bestPrice - decreaseStep
		if minPrice > 0 && target < minPrice {
			target = minPrice
			if target >= curPrice {
				return priceDecision{
					NewPrice: -1,
					Reason:   fmt.Sprintf("竞品低价 R%d，已触碰底价保护 R%d", bestPrice, minPrice),
				}
			}
			return priceDecision{
				NewPrice: target,
				Action:   "底价保护",
				Reason:   fmt.Sprintf("竞品低价 R%d，触碰底价保护 R%d", bestPrice, minPrice),
			}
		}
		return priceDecision{
			NewPrice: target,
			Action:   "跟降",
			Reason:   fmt.Sprintf("竞品低价 R%d，跟降 R%d 至 R%d", bestPrice, decreaseStep, target),
		}
	}

	return priceDecision{NewPrice: -1}
}
