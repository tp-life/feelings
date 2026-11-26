package composer

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// SynthesizeBGMPreset 根据预设 ID 程序化合成温暖克制的怀旧配乐轨（零版权风险、无需外部音频文件依赖）。
// 返回值 hasBGM=false 表示用户选择了 "none"（纯人声+环境声景，无背景音乐）。
func SynthesizeBGMPreset(ctx context.Context, preset string, totalDur float64, outPath string) (bool, error) {
	if totalDur <= 0.5 {
		totalDur = 5.0
	}
	fadeOut := totalDur - 1.2
	if fadeOut < 0.2 {
		fadeOut = 0.2
	}

	preset = strings.TrimSpace(preset)
	if preset == "none" {
		cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-f", "lavfi",
			"-i", fmt.Sprintf("anullsrc=r=44100:cl=stereo:d=%.2f", totalDur),
			outPath,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			return false, fmt.Errorf("generate silent bgm failed: %w: %s", err, trimErr(string(out)))
		}
		return false, nil
	}

	var expr string
	var af string

	switch preset {
	case "tape_piano":
		// 四小节慢速旧磁带电钢泛音进行（Cmaj7 -> Am7 -> Fmaj7 -> G6，每 4 秒柔和呼吸切换 + 轻微磁带颤音与空间回声）
		expr = "0.075*(0.55+0.45*cos(2*PI*0.25*t))*(" +
			"if(lt(mod(t,16),4), sin(2*PI*261.63*t)+0.6*sin(2*PI*329.63*t)+0.45*sin(2*PI*392.00*t)+0.35*sin(2*PI*493.88*t)," +
			"if(lt(mod(t,16),8), sin(2*PI*220.00*t)+0.6*sin(2*PI*261.63*t)+0.45*sin(2*PI*329.63*t)+0.35*sin(2*PI*392.00*t)," +
			"if(lt(mod(t,16),12), sin(2*PI*174.61*t)+0.6*sin(2*PI*220.00*t)+0.45*sin(2*PI*261.63*t)+0.35*sin(2*PI*329.63*t)," +
			"sin(2*PI*196.00*t)+0.6*sin(2*PI*246.94*t)+0.45*sin(2*PI*293.66*t)+0.35*sin(2*PI*329.63*t)))))"
		af = fmt.Sprintf("lowpass=f=1250,highpass=f=85,vibrato=f=3.6:d=0.05,aecho=0.8:0.84:190:0.22,afade=t=in:st=0:d=1.0,afade=t=out:st=%.2f:d=1.2", fadeOut)

	case "acoustic_breeze":
		// 午后木吉他空弦五度分解泛音（G大调温暖空弦共鸣 + 轻缓拍频呼吸）
		expr = "0.07*exp(-1.4*mod(t,2.0))*(sin(2*PI*196.00*t)+0.5*sin(2*PI*392.00*t))+" +
			"0.055*exp(-1.5*mod(t+0.66,2.0))*(sin(2*PI*293.66*t)+0.4*sin(2*PI*587.33*t))+" +
			"0.048*exp(-1.6*mod(t+1.33,2.0))*(sin(2*PI*329.63*t)+0.35*sin(2*PI*493.88*t))"
		af = fmt.Sprintf("lowpass=f=1650,highpass=f=95,aecho=0.8:0.86:240:0.26,afade=t=in:st=0:d=0.9,afade=t=out:st=%.2f:d=1.2", fadeOut)

	case "music_box":
		// 童年八音盒清亮钟琴泛音（C5-E5-G5-D5-A4 慢速颗粒泛音 + 柔和余响）
		expr = "0.055*exp(-2.6*mod(t,1.6))*sin(2*PI*523.25*t)+" +
			"0.048*exp(-2.6*mod(t+0.8,1.6))*sin(2*PI*659.25*t)+" +
			"0.042*exp(-2.4*mod(t+0.4,2.4))*sin(2*PI*783.99*t)+" +
			"0.045*exp(-2.2*mod(t+1.2,2.4))*sin(2*PI*587.33*t)"
		af = fmt.Sprintf("lowpass=f=2800,highpass=f=220,aecho=0.8:0.88:280:0.28,afade=t=in:st=0:d=0.8,afade=t=out:st=%.2f:d=1.2", fadeOut)

	default: // "warm_pad"
		// C大九和弦极轻柔和低频温暖铺底（C3=130.81, G3=196.00, D4=293.66, E4=329.63）
		expr = "0.06*sin(2*PI*130.81*t)+0.045*sin(2*PI*196.00*t)+0.03*sin(2*PI*293.66*t)*0.5*(1+sin(2*PI*0.18*t))+0.025*sin(2*PI*329.63*t)*0.5*(1+cos(2*PI*0.13*t))"
		af = fmt.Sprintf("lowpass=f=680,afade=t=in:st=0:d=1.0,afade=t=out:st=%.2f:d=1.2", fadeOut)
	}

	args := []string{
		"-y",
		"-f", "lavfi",
		"-i", fmt.Sprintf("aevalsrc='%s':s=44100:d=%.2f", expr, totalDur),
		"-af", af,
		"-ac", "2",
		outPath,
	}
	if out, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput(); err != nil {
		return false, fmt.Errorf("ffmpeg bgm preset (%s) failed: %w: %s", preset, err, trimErr(string(out)))
	}
	return true, nil
}
