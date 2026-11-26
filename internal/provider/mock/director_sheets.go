package mock

import (
	"fmt"

	"feelings-studio/internal/prompting"
	"feelings-studio/internal/provider"
)

// enrichDirectorSheets 为 Mock 分镜自动注入工业级导演时序详版（资产锚定、参考图边界规则、
// 首尾帧衔接、2~3段情怀呼吸节拍、硬性输出约束、以及物理隔离的后期心理字幕与转场设计）。
func enrichDirectorSheets(drafts []provider.ShotDraft, assets []provider.AssetDraft, allowSummerDemo bool) []provider.ShotDraft {
	for i := range drafts {
		if len(drafts[i].DirectorSheet.TimelineBeats) > 0 {
			continue
		}
		if allowSummerDemo {
			if custom, ok := summer1998DirectorSheet(drafts[i].ShotID, drafts[i]); ok {
				drafts[i].DirectorSheet = custom
				continue
			}
		}
		drafts[i].DirectorSheet = buildDefaultDirectorSheet(i+1, drafts[i], assets)
	}
	return drafts
}

func summer1998DirectorSheet(shotID string, d provider.ShotDraft) (prompting.DirectorSheetSpec, bool) {
	dur := float64(d.DurationSec)
	if dur <= 0 {
		dur = 5
	}
	switch shotID {
	case "g1":
		return prompting.DirectorSheetSpec{
			ShotTitle: prompting.BilingualText{ZH: "午后巷口小卖部 · 感官序幕", EN: "Afternoon Alley Corner Store Opening"},
			AssetBindings: []prompting.AssetBinding{
				{
					Role: "scene", AssetName: "小卖部内部", RefToken: "{{Asset 2}}",
					UsageRule:   prompting.BilingualText{ZH: "只参考门内旧冰柜与木货架的年代材质与色彩关系；不参考室内正面构图", EN: "Reference vintage freezer and shelf textures only; do not copy interior front framing"},
					ExcludeHint: prompting.BilingualText{ZH: "本镜为外景空镜，勿出现 {{Asset 3}} 男孩或 {{Asset 1}} 老板近景", EN: "Exterior establishing shot; do not place boy or shopkeeper in foreground"},
				},
			},
			CastSummary:       prompting.BilingualText{ZH: "无主角近景，仅远处巷尾有街坊慢行背影。", EN: "No foreground cast; only distant neighbors strolling quietly."},
			BackgroundSetting: prompting.BilingualText{ZH: "1990年代北方老青砖胡同口小卖部，午后两点明朗暖黄斜阳，地面投下清晰屋檐长影，门口敞开木门旁摆着绿玻璃汽水箱与手写价格牌。", EN: "1990s brick alley corner store under warm 2pm sunlight casting crisp diagonal shadows, green glass soda crate beside open wooden doors."},
			ContinuitySummary: prompting.BilingualText{ZH: "当前分镜为第1镜开场：通过午后热风、蝉鸣与敞开的木门建立1998年夏天的时空坐标，不急于切入人物，留出观众情绪沉静入口。", EN: "Shot 1 establishes the 1998 summer atmosphere before introducing characters."},
			FirstFrameLink:    prompting.BilingualText{ZH: "画面第一帧由黑场轻柔淡入至午后阳光斜照的胡同小卖部全貌，保持高光通透不刺眼。", EN: "Fades in softly from black onto the sunlit alley corner store."},
			LastFrameLink:     prompting.BilingualText{ZH: "尾帧推进至更靠近敞开木门与门内微暗空间，为第2镜切入柜台玻璃糖罐微距做好明暗过渡。", EN: "Ends slightly closer to the open wooden doorway, preparing the light transition into Shot 2."},
			TimelineBeats: []prompting.TimelineBeat{
				{
					StartSec: 0.0, EndSec: 2.2,
					Title:        prompting.BilingualText{ZH: "午后热浪与巷口静谧建立", EN: "Afternoon Heat & Alley Stillness"},
					Visual:       prompting.BilingualText{ZH: "阳光斜洒在斑驳水泥墙面与红箱绿汽水瓶上，远处晾晒的白衬衫被微风轻轻吹动，空气中细微阳光浮尘缓缓飘动。", EN: "Warm diagonal sunlight rests on concrete walls and green soda bottles while distant hanging shirts sway softly."},
					Dialogue:     prompting.BilingualText{ZH: "无", EN: "None"},
					Forbid:       prompting.BilingualText{ZH: "禁止出现现代汽车、智能手机、快递袋、LED招牌；禁止任何文字或字幕；禁止人物突然闯入近景", EN: "No modern cars, smartphones, LED signs, text, or foreground figures"},
					Constraint:   prompting.BilingualText{ZH: "木门保持敞开静止，门口汽水瓶数量与位置不变，光向稳定来自左上方", EN: "Wooden door stays open and still, bottle crate unchanged, consistent upper-left sunlight"},
					Staging:      prompting.BilingualText{ZH: "小卖部位于画面右侧三分之二，左侧保留纵深巷道引导视线", EN: "Corner store occupies right two-thirds, deep alley leads eye on the left"},
					Camera:       prompting.BilingualText{ZH: "极缓慢平稳前推，几乎无晃动", EN: "Ultra-slow steady forward push-in"},
					SFX:          prompting.BilingualText{ZH: "由远及近的夏日蝉鸣，伴随轻微热风吹过树梢声", EN: "Summer cicadas swelling gently with warm alley breeze"},
					PsychMapping: "普鲁斯特感官线索先行（光影+蝉鸣），瞬间激活自传体童年记忆",
				},
				{
					StartSec: 2.2, EndSec: dur,
					Title:        prompting.BilingualText{ZH: "走向敞开木门的呼吸留白", EN: "Approaching Doorway Breathing Pause"},
					Visual:       prompting.BilingualText{ZH: "镜头缓缓逼近木门与露出一角的白色老冰柜，远处巷尾行人背影慢步淡出视线，画面底部保持干净水泥地留白。", EN: "Camera drifts closer to the open wooden door and white chest freezer while clean sunlit pavement anchors the lower frame."},
					Dialogue:     prompting.BilingualText{ZH: "无", EN: "None"},
					Forbid:       prompting.BilingualText{ZH: "禁止曝光闪烁、镜头突然加速、墙面纹理扭曲融化", EN: "No exposure flicker, sudden speed jump, or wall warping"},
					Constraint:   prompting.BilingualText{ZH: "最后0.5秒运镜柔和缓停，下方留出约18%干净空间供后期字幕淡入", EN: "Ease-out settle in final 0.5s with clean lower 18% subtitle safe area"},
					Camera:       prompting.BilingualText{ZH: "匀速极缓推进，最后0.5秒缓停", EN: "Slow push-in easing gently to rest"},
					SFX:          prompting.BilingualText{ZH: "蝉鸣平稳铺底，隐约透出老冰柜极轻低频嗡鸣", EN: "Steady cicadas with faint distant freezer hum"},
					PsychMapping: "降低视觉信息密度，配合克制旁白触发温和代入感",
				},
			},
			OutputConstraints: []prompting.BilingualText{
				{ZH: "全程保持1990年代北方胡同建筑与物件考据，严禁现代招牌与服装穿帮", EN: "Strict 1990s Chinese neighborhood authenticity with zero modern props"},
				{ZH: "画面中严禁生成任何可读文字、数字或水印，所有文字一律后期压字", EN: "No rendered text, digits, or watermarks; subtitles added strictly in post"},
				{ZH: "光线方向全程统一为午后暖黄斜阳，无闪频与色彩跳变", EN: "Consistent afternoon warm directional sunlight with zero flicker"},
			},
			PostProduction: prompting.PostProductionNotes{
				SubtitleStrategy: "本镜旁白仅14字，全部由 FFmpeg 后期压制极简暖白字幕，不在生成画面中出现任何文字。",
				SubtitleRhythm:   []string{"0.5s-4.4s 淡入旁白：「夏天的蝉鸣，从小卖部木门开始。」"},
				TransitionDesign: prompting.TransitionDesignSpec{
					FromPrev: "片头黑场 0.45s 柔缓淡入",
					ToNext:   "0.40s 柔和叠化切入第2镜玻璃糖罐特写（保持暖色高光延续）",
				},
			},
		}, true

	case "g5":
		return prompting.DirectorSheetSpec{
			ShotTitle: prompting.BilingualText{ZH: "关系微光 · 老板微笑递糖", EN: "Relational Warmth · Shopkeeper Handing Candy"},
			AssetBindings: []prompting.AssetBinding{
				{
					Role: "character", AssetName: "男孩", RefToken: "{{Asset 3}}",
					UsageRule:   prompting.BilingualText{ZH: "只参考寸头面容、微旧纯白T恤与深蓝短裤特征；不参考正面直视镜头的站姿", EN: "Reference boy's face, buzz cut, white tee, and navy shorts only; do not copy front-facing pose"},
					ExcludeHint: prompting.BilingualText{ZH: "勿换成 {{Asset 4}} 蓝衣小伙伴", EN: "Do not swap with buddy in light blue shirt"},
				},
				{
					Role: "character", AssetName: "小卖店老板", RefToken: "{{Asset 1}}",
					UsageRule: prompting.BilingualText{ZH: "只参考40多岁慈祥面容、短黑发与浅灰棉T恤；保持温和微笑", EN: "Reference 40s kind shopkeeper face, short hair, and grey tee"},
				},
				{
					Role: "scene", AssetName: "小卖部内部", RefToken: "{{Asset 2}}",
					UsageRule: prompting.BilingualText{ZH: "只参考陈旧木柜台、玻璃糖罐、铁皮零钱盒与货架材质；不参考空景广角构图", EN: "Reference worn wooden counter, glass candy jars, tin cash box, and shelves"},
				},
			},
			CastSummary:       prompting.BilingualText{ZH: "白衣8岁男孩（画面左侧，侧身微仰头）与灰衣小卖部老板（画面右侧，隔柜台微倾微笑）。画面仅此2人。", EN: "8-year-old boy on left and shopkeeper on right across wooden counter. Strictly 2 people."},
			BackgroundSetting: prompting.BilingualText{ZH: "90年代小卖部内部木柜台前，左侧敞开木门透入柔和午后自然光，柜台上排列玻璃糖罐与旧铁皮零钱盒，背景货架自然虚化。", EN: "1990s corner store counter with soft doorway daylight from left, glass candy jars and tin box on wood."},
			ContinuitySummary: prompting.BilingualText{ZH: "当前分镜为全片情感高点第5镜，承接第4镜男孩掌心数硬币：展现“被长辈无条件善待”的童年微光瞬间，克制温暖而不煽情。", EN: "Shot 5 is the emotional peak following Shot 4's coin counting, capturing quiet unconditional kindness."},
			FirstFrameLink:    prompting.BilingualText{ZH: "首帧承接第4镜柜台室内柔和漫射光色温，两人已站在木柜台两侧，老板指尖正捏着一颗红色糖纸水果糖。", EN: "Matches Shot 4's warm diffused indoor tone; shopkeeper holds a red-wrapped candy across the counter."},
			LastFrameLink:     prompting.BilingualText{ZH: "尾帧男孩已接住糖果并露出克制腼腆的微笑，光线稳定，为第6镜转至阳光明媚的巷口分冰棍做好情绪上扬衔接。", EN: "Ends as boy receives the candy with a shy smile, bridging warmly into Shot 6 outside."},
			TimelineBeats: []prompting.TimelineBeat{
				{
					StartSec: 0.0, EndSec: 2.0,
					Title:        prompting.BilingualText{ZH: "微笑微倾与递糖动作", EN: "Gentle Lean & Candy Handoff"},
					Visual:       prompting.BilingualText{ZH: "老板眼角笑意柔和加深，右手极其缓慢平稳地将红色糖纸水果糖递向男孩掌心；男孩专注仰望老板，右手轻缓伸出接糖。", EN: "Shopkeeper smiles warmly extending a red-wrapped candy slowly as the boy looks up and reaches carefully."},
					Dialogue:     prompting.BilingualText{ZH: "无（禁止两人张嘴说话）", EN: "None (no talking mouth motion)"},
					Forbid:       prompting.BilingualText{ZH: "禁止手指粘连变形、糖果凭空变大或变色、人物转头看镜头、夸张咧嘴大笑", EN: "No warped fingers, morphing candy, looking at camera, or exaggerated grin"},
					Constraint:   prompting.BilingualText{ZH: "柜台上的玻璃糖罐与铁皮盒位置完全静止，两人站位不移动，只做上身与手部微动", EN: "Jars and tin box stay completely still; only gentle hand and subtle upper-body motion"},
					Staging:      prompting.BilingualText{ZH: "男孩在左面朝右，老板在右面朝左，递糖双手处于画面中央黄金焦点", EN: "Boy on left facing right, shopkeeper on right facing left, hands meet at center focal point"},
					Camera:       prompting.BilingualText{ZH: "极缓慢微推近两人递糖的双手与侧脸神情", EN: "Ultra-slow gentle push-in toward their hands and profiles"},
					SFX:          prompting.BilingualText{ZH: "极轻的糖纸细微沙沙声，背景巷口微风轻响", EN: "Delicate candy wrapper crinkle over quiet alley room tone"},
					PsychMapping: "社会联结与信任锚点：童年被善待的微小瞬间，直接触发温暖治愈感",
				},
				{
					StartSec: 2.0, EndSec: 3.8,
					Title:        prompting.BilingualText{ZH: "接糖瞬间与眼神微亮", EN: "Receiving Candy & Warm Glance"},
					Visual:       prompting.BilingualText{ZH: "男孩指尖小心握住水果糖，嘴角缓缓扬起一丝克制腼腆的笑意；老板手指轻轻松开并微微点头，门外暖光微尘在柜台上方轻浮。", EN: "Boy closes his fingers gently around the candy with a shy smile as shopkeeper releases softly; sunlit dust motes drift above."},
					Dialogue:     prompting.BilingualText{ZH: "无", EN: "None"},
					Forbid:       prompting.BilingualText{ZH: "禁止面部五官闪烁漂移、禁止出现第三个人物、禁止现代标签出现", EN: "No facial drift, third person entering, or modern labels"},
					Constraint:   prompting.BilingualText{ZH: "始终只有1颗水果糖，不增加数量；人物表情自然克制", EN: "Strictly one candy; restrained authentic expressions"},
					Camera:       prompting.BilingualText{ZH: "极缓推镜继续微进后接近静止", EN: "Slow push-in tapering toward stillness"},
					SFX:          prompting.BilingualText{ZH: "环境底噪温柔托底，保留轻微安静呼吸感", EN: "Warm quiet interior ambience"},
					PsychMapping: "情绪共情高点，低防御心理敞开",
				},
				{
					StartSec: 3.8, EndSec: dur,
					Title:        prompting.BilingualText{ZH: "温暖定格留白", EN: "Tender Settling Pause"},
					Visual:       prompting.BilingualText{ZH: "画面缓停在两人温和对视的余韵中，门外暖光稳定洒在木柜台与玻璃罐壁上，下方保留干净木柜台留白。", EN: "Scene settles warmly on their quiet mutual smile as soft daylight holds steady across the counter."},
					Dialogue:     prompting.BilingualText{ZH: "无", EN: "None"},
					Forbid:       prompting.BilingualText{ZH: "禁止镜头突然回拉、禁止任何画面文字", EN: "No sudden pullback or rendered text"},
					Constraint:   prompting.BilingualText{ZH: "结尾平稳柔和，供后期暖白字幕收尾", EN: "Calm settle ready for subtitle fade-out"},
					Camera:       prompting.BilingualText{ZH: "缓停固定", EN: "Ease-out to static hold"},
					SFX:          prompting.BilingualText{ZH: "门外隐约一声夏风拂过", EN: "Soft summer breeze outside"},
					PsychMapping: "留白呼吸区，让观众投射自己记忆里的温柔长辈",
				},
			},
			OutputConstraints: []prompting.BilingualText{
				{ZH: "人物锁定为男孩与老板共2人，外貌服装严格与 {{Asset 3}}、{{Asset 1}} 一致", EN: "Strictly 2 characters matching Asset 3 and Asset 1"},
				{ZH: "全程严禁张嘴说话或直视镜头，保持纪实偷拍般自然克制", EN: "No lip movement or looking at camera; candid documentary restraint"},
				{ZH: "画面中禁止任何可读汉字、数字、商标或字幕", EN: "Zero readable text, logos, or subtitles in generated video"},
			},
			PostProduction: prompting.PostProductionNotes{
				SubtitleStrategy: "情绪高点切忌过度煽情，只用13字陈述具体善意动作，由后期压字。",
				SubtitleRhythm:   []string{"0.45s-4.4s 淡入旁白：「老板笑着，顺手多递来一颗糖。」"},
				TransitionDesign: prompting.TransitionDesignSpec{
					FromPrev: "承接第4镜掌心硬币特写，0.40s 柔和叠化切出双人中景",
					ToNext:   "0.40s 暖光叠化切至第6镜阳光胡同分冰棍",
				},
			},
		}, true

	case "g10":
		return prompting.DirectorSheetSpec{
			ShotTitle: prompting.BilingualText{ZH: "时空余韵 · 黄昏空凳小卖部", EN: "Liminal Closure · Golden Hour Empty Stools"},
			AssetBindings: []prompting.AssetBinding{
				{
					Role: "scene", AssetName: "小卖部内部", RefToken: "{{Asset 2}}",
					UsageRule:   prompting.BilingualText{ZH: "只参考门口老白冰柜与木门框材质，保持与第1镜同一栋建筑结构", EN: "Match Shot 1 architecture and doorway freezer texture"},
					ExcludeHint: prompting.BilingualText{ZH: "结尾空镜无人出镜，严禁出现任何角色", EN: "Empty contemplative closing shot; no characters"},
				},
			},
			CastSummary:       prompting.BilingualText{ZH: "无人物出镜（纯空镜留白）。", EN: "No characters (pure contemplative empty frame)."},
			BackgroundSetting: prompting.BilingualText{ZH: "记忆中琥珀金黄昏下的90年代老小卖部外景，木门敞开，门前静放两张空小木凳、一辆老二八自行车与绿汽水箱，上方绿树垂枝。", EN: "Nostalgic 1990s corner store at amber golden hour with two empty wooden stools, vintage bicycle, and overhanging green branches."},
			ContinuitySummary: prompting.BilingualText{ZH: "当前分镜为全片第10镜收尾，承接第9镜成年翻看相册：回到记忆中最温暖安宁的黄昏瞬间，完成怀旧救赎闭环并引导评论区共鸣。", EN: "Shot 10 closes the nostalgic redemptive arc after Shot 9's photo album, offering serene emotional closure."},
			FirstFrameLink:    prompting.BilingualText{ZH: "首帧由第9镜旧相册暖黄色纸页柔和叠化切入，无缝承接琥珀夕阳色调。", EN: "Dissolves seamlessly from Shot 9's warm yellowed photo album into golden-hour dusk."},
			LastFrameLink:     prompting.BilingualText{ZH: "尾帧画面极度安宁平稳，下方大面积夕阳水泥路面留白，供结尾开放式提问字幕停留后缓慢淡入黑场。", EN: "Ends in deep serene stillness with clean sunlit foreground for the closing question and fade-out."},
			TimelineBeats: []prompting.TimelineBeat{
				{
					StartSec: 0.0, EndSec: 3.2,
					Title:        prompting.BilingualText{ZH: "晚风拂枝与夕阳树影轻晃", EN: "Evening Breeze & Golden Leaf Shadows"},
					Visual:       prompting.BilingualText{ZH: "低角度琥珀金夕阳温暖包裹水泥墙面与两张空木凳，上方垂落的绿树枝叶被晚风轻轻吹动，墙面树影柔和慢晃，金色微尘缓缓浮沉。", EN: "Amber sunset wraps the concrete walls and two empty wooden stools as overhead branches sway gently and leaf shadows dance softly."},
					Dialogue:     prompting.BilingualText{ZH: "无", EN: "None"},
					Forbid:       prompting.BilingualText{ZH: "禁止出现人物、现代车辆、文字、剧烈风吹或光线闪烁", EN: "No people, modern vehicles, text, harsh wind, or light flicker"},
					Constraint:   prompting.BilingualText{ZH: "两张空木凳、自行车与汽水瓶完全静止不动，只靠晚风枝叶、树影与极缓镜头运动传递时间感", EN: "Stools, bicycle, and bottles stay completely still; motion lives purely in leaves, shadows, and gentle camera pull-out"},
					Camera:       prompting.BilingualText{ZH: "极缓慢平稳向后拉远，温柔拉开时空距离", EN: "Ultra-slow steady camera pull-out evoking tender temporal distance"},
					SFX:          prompting.BilingualText{ZH: "温柔夏夜虫鸣与晚风拂叶声轻柔铺满", EN: "Gentle evening crickets and soft breeze through leaves"},
					PsychMapping: "怀旧救赎曲线落点：苦乐参半转化为温和安宁的自我连续性慰藉",
				},
				{
					StartSec: 3.2, EndSec: dur,
					Title:        prompting.BilingualText{ZH: "静谧定格与开放式余韵", EN: "Peaceful Hold & Open Closure"},
					Visual:       prompting.BilingualText{ZH: "镜头缓慢停住，夕阳暖光沉静稳定，两张空凳在金光里仿佛仍在等候放学的孩子，画面下方三分之一为干净暖光地面留白。", EN: "Camera eases to rest as golden dusk holds peacefully over the empty stools with clean sunlit lower third."},
					Dialogue:     prompting.BilingualText{ZH: "无", EN: "None"},
					Forbid:       prompting.BilingualText{ZH: "禁止镜头漂移、高光突变、出现任何可读文字", EN: "No camera drift, brightness jump, or rendered text"},
					Constraint:   prompting.BilingualText{ZH: "最后1秒几乎完全静止，方便后期字幕停留与黑场淡出", EN: "Nearly static final second ready for closing subtitle and fade to black"},
					Camera:       prompting.BilingualText{ZH: "缓停固定定格", EN: "Ease-out to locked stillness"},
					SFX:          prompting.BilingualText{ZH: "夏夜虫鸣伴随温暖和弦余音渐弱收干", EN: "Crickets and warm chord pad gently tapering out"},
					PsychMapping: "社交货币与评论区钩子：留白触发观众回答“我家巷口的小卖部”",
				},
			},
			OutputConstraints: []prompting.BilingualText{
				{ZH: "全程无人出镜，保持空木凳与旧小卖部的宁静守候感", EN: "Strictly zero people; preserve serene empty-stool stillness"},
				{ZH: "夕阳暖金光向全程稳定，无曝光跳变与画面闪烁", EN: "Steady warm golden-hour lighting with zero exposure jump"},
				{ZH: "画面中严禁生成文字、字幕或水印，尾句提问仅在后期压字", EN: "No generated text or watermark; closing hook added strictly in post"},
			},
			PostProduction: prompting.PostProductionNotes{
				SubtitleStrategy: "尾镜只放一句轻量开放式回忆提问（不硬广求赞），配合0.75s黑场淡出制造强烈余韵。",
				SubtitleRhythm:   []string{"0.6s-5.2s 暖白极简字幕：「你家巷口的那间小卖部，现在还在吗？」"},
				TransitionDesign: prompting.TransitionDesignSpec{
					FromPrev: "承接第9镜旧相册暖黄画面，0.45s 柔和叠化切入记忆黄昏",
					ToOutro:  "5.25s 起 0.75s 缓慢黑场淡出，BGM与虫鸣同步缓落收尾",
				},
			},
		}, true
	}
	return prompting.DirectorSheetSpec{}, false
}

func buildDefaultDirectorSheet(order int, d provider.ShotDraft, assets []provider.AssetDraft) prompting.DirectorSheetSpec {
	dur := float64(d.DurationSec)
	if dur <= 0 {
		dur = 5
	}
	mid := dur * 0.62
	transitionToNext := "0.40s 柔和叠化衔接下一镜"
	if d.I2VSpec.TransitionMode == prompting.TransitionFirstLastFrame {
		transitionToNext = "首尾帧平滑过渡衔接下一镜（" + d.I2VSpec.TransitionHint.ZH + "）"
	}

	// 按该镜叙事阶段自动绑定当前项目真实资产（空镜开场/收尾优先绑场景，仪式绑道具+场景，关系高点绑人物+场景）
	var bindings []prompting.AssetBinding
	for idx, a := range assets {
		include := false
		switch d.Phase {
		case "sensory_hook":
			include = a.Role == "scene"
		case "concrete_ritual":
			include = a.Role == "prop" || a.Role == "scene"
		case "relational_warmth":
			include = a.Role == "character" || a.Role == "scene"
		case "liminal_closure":
			include = a.Role == "scene" || a.Role == "prop"
		default:
			include = idx == 0
		}
		if !include {
			continue
		}
		ruleZH := "只参考外貌特征、服装款式与年代材质；不参考站姿、构图角度与数量"
		ruleEN := "Reference appearance, outfit, and era texture only; do not copy pose, angle, or count"
		if a.Role == "scene" {
			ruleZH = "只参考空间材质、建筑结构与光影氛围；不参考空镜正面构图"
			ruleEN = "Reference spatial textures, architecture, and lighting mood only; do not copy framing"
		} else if a.Role == "prop" {
			ruleZH = "只参考道具材质、形状比例与磨损质感；不参考摆放角度与数量"
			ruleEN = "Reference prop shape, proportions, and aged texture only; do not copy angle or quantity"
		}
		bindings = append(bindings, prompting.AssetBinding{
			AssetID:   a.ID,
			Role:      a.Role,
			AssetName: a.Name,
			RefToken:  fmt.Sprintf("{{Asset %d}}", idx+1),
			UsageRule: prompting.BilingualText{ZH: ruleZH, EN: ruleEN},
		})
	}

	return prompting.DirectorSheetSpec{
		ShotTitle:         prompting.BilingualText{ZH: fmt.Sprintf("%s · %s", d.ShotID, d.Phase), EN: fmt.Sprintf("%s · %s", d.ShotID, d.Phase)},
		AssetBindings:     bindings,
		CastSummary:       prompting.BilingualText{ZH: d.T2ISpec.SubjectAnchor.ZH, EN: d.T2ISpec.SubjectAnchor.EN},
		BackgroundSetting: prompting.BilingualText{ZH: d.T2ISpec.EraEnvironment.ZH + "；" + d.T2ISpec.LightingMood.ZH, EN: d.T2ISpec.EraEnvironment.EN + ". " + d.T2ISpec.LightingMood.EN},
		ContinuitySummary: prompting.BilingualText{ZH: fmt.Sprintf("第%d镜（%s）：%s", order, d.Phase, d.PsychTrigger), EN: fmt.Sprintf("Shot %d (%s): %s", order, d.Phase, d.PsychTrigger)},
		FirstFrameLink:    prompting.BilingualText{ZH: "首帧保持与上一镜一致的胶片暖调与自然光向。", EN: "First frame matches previous shot's film palette and natural light direction."},
		LastFrameLink:     prompting.BilingualText{ZH: "尾帧动作平稳缓停，下方保留约18%干净留白区供后期极简字幕浮现。", EN: "Last frame settles smoothly with clean lower 18% breathing space for post subtitles."},
		TimelineBeats: []prompting.TimelineBeat{
			{
				StartSec: 0, EndSec: mid,
				Title:        prompting.BilingualText{ZH: "主体微动与氛围展开", EN: "Micro-Action & Atmosphere"},
				Visual:       prompting.BilingualText{ZH: d.I2VSpec.SubjectMicroAction.ZH + "；" + d.I2VSpec.EnvironmentPhysics.ZH, EN: d.I2VSpec.SubjectMicroAction.EN + ". " + d.I2VSpec.EnvironmentPhysics.EN},
				Dialogue:     prompting.BilingualText{ZH: "无", EN: "None"},
				Forbid:       prompting.BilingualText{ZH: "禁止人物直视镜头或张嘴说话；禁止出现现代穿帮物、文字或畸形手指", EN: "No looking at camera, talking mouths, modern props, text, or warped hands"},
				Constraint:   prompting.BilingualText{ZH: "主体外貌、服装与场景道具数量全程锁定不变，动作自然克制", EN: "Lock subject appearance, outfit, and prop count; restrained physical motion"},
				Camera:       prompting.BilingualText{ZH: d.I2VSpec.CameraDetail.ZH, EN: d.I2VSpec.CameraDetail.EN},
				SFX:          prompting.BilingualText{ZH: "柔和环境声景（" + d.AmbientTag + "）", EN: "Soft ambient soundscape (" + d.AmbientTag + ")"},
				PsychMapping: d.PsychTrigger,
			},
			{
				StartSec: mid, EndSec: dur,
				Title:        prompting.BilingualText{ZH: "情绪停顿与留白余韵", EN: "Breathing Pause & Lingering Stillness"},
				Visual:       prompting.BilingualText{ZH: d.I2VSpec.EnvironmentPhysics.ZH + "，主体动作缓缓收住，画面沉静留白。", EN: d.I2VSpec.EnvironmentPhysics.EN + ", subject motion eases gently into quiet stillness."},
				Dialogue:     prompting.BilingualText{ZH: "无", EN: "None"},
				Forbid:       prompting.BilingualText{ZH: "禁止镜头漂移、高光闪烁或画面文字", EN: "No camera drift, exposure flicker, or rendered text"},
				Constraint:   prompting.BilingualText{ZH: "结尾最后0.5秒柔和缓停，保持构图稳定", EN: "Ease-out settle in final 0.5s with steady framing"},
				Camera:       prompting.BilingualText{ZH: "极缓收势定格", EN: "Gentle ease-out hold"},
				SFX:          prompting.BilingualText{ZH: "环境底噪延续渐轻", EN: "Ambient tone tapering softly"},
				PsychMapping: "预留 ≥25% 无旁白呼吸空间，增强自我投射与沉浸感",
			},
		},
		OutputConstraints: []prompting.BilingualText{
			{ZH: "全程不出现任何可读文字、数字、Logo或水印，字幕统一后期压制", EN: "Zero readable text, digits, logos, or watermarks; subtitles added in post"},
			{ZH: "保持人物外貌、服装与环境光向全程一致，避免五官与背景变形", EN: "Maintain consistent character identity, outfit, and lighting without morphing"},
		},
		PostProduction: prompting.PostProductionNotes{
			SubtitleStrategy: "所有旁白文字一律后期 FFmpeg 压制，不写入视频生成指令。",
			SubtitleRhythm:   []string{fmt.Sprintf("0.45s-%.1fs 暖白极简字幕：「%s」", dur-0.55, d.Narration)},
			TransitionDesign: prompting.TransitionDesignSpec{
				FromPrev: "0.40s 柔和叠化承接上一镜",
				ToNext:   transitionToNext,
			},
		},
	}
}
