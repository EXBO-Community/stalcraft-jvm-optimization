package main

import "fmt"

func generateFlagsFromConfig(cfg Config) []string {
	cc := cfg.ReservedCodeCacheSizeMB
	if cc == 0 {
		cc = 256
	}

	flags := []string{
		fmt.Sprintf("-Xmx%dg", cfg.HeapSizeGB),
		fmt.Sprintf("-Xms%dg", cfg.HeapSizeGB),

		fmt.Sprintf("-XX:MetaspaceSize=%dm", cfg.MetaspaceMB),
		fmt.Sprintf("-XX:MaxMetaspaceSize=%dm", cfg.MetaspaceMB),

		"-XX:+UseG1GC",
		"-XX:+UnlockExperimentalVMOptions",
		fmt.Sprintf("-XX:MaxGCPauseMillis=%d", cfg.MaxGCPauseMillis),
		fmt.Sprintf("-XX:G1HeapRegionSize=%dm", cfg.G1HeapRegionSizeMB),
		fmt.Sprintf("-XX:G1NewSizePercent=%d", cfg.G1NewSizePercent),
		fmt.Sprintf("-XX:G1MaxNewSizePercent=%d", cfg.G1MaxNewSizePercent),
		fmt.Sprintf("-XX:G1ReservePercent=%d", cfg.G1ReservePercent),
		fmt.Sprintf("-XX:G1HeapWastePercent=%d", cfg.G1HeapWastePercent),
		fmt.Sprintf("-XX:G1MixedGCCountTarget=%d", cfg.G1MixedGCCountTarget),
		"-XX:+G1UseAdaptiveIHOP",
		fmt.Sprintf("-XX:InitiatingHeapOccupancyPercent=%d", cfg.InitiatingHeapOccupancyPercent),
		fmt.Sprintf("-XX:G1MixedGCLiveThresholdPercent=%d", cfg.G1MixedGCLiveThresholdPercent),
		fmt.Sprintf("-XX:G1RSetUpdatingPauseTimePercent=%d", cfg.G1RSetUpdatingPauseTimePercent),
		fmt.Sprintf("-XX:SurvivorRatio=%d", cfg.SurvivorRatio),
		fmt.Sprintf("-XX:MaxTenuringThreshold=%d", cfg.MaxTenuringThreshold),

		fmt.Sprintf("-XX:ParallelGCThreads=%d", cfg.ParallelGCThreads),
		fmt.Sprintf("-XX:ConcGCThreads=%d", cfg.ConcGCThreads),

		"-XX:+ParallelRefProcEnabled",
		"-XX:+DisableExplicitGC",
		fmt.Sprintf("-XX:SoftRefLRUPolicyMSPerMB=%d", cfg.SoftRefLRUPolicyMSPerMB),

		"-XX:-UseBiasedLocking",
		"-XX:+DisableAttachMechanism",

		fmt.Sprintf("-XX:ReservedCodeCacheSize=%dm", cc),
		fmt.Sprintf("-XX:NonNMethodCodeHeapSize=%dm", cc*5/100),
		fmt.Sprintf("-XX:ProfiledCodeHeapSize=%dm", cc*48/100),
		fmt.Sprintf("-XX:NonProfiledCodeHeapSize=%dm", cc-cc*5/100-cc*48/100),
		fmt.Sprintf("-XX:MaxInlineLevel=%d", cfg.MaxInlineLevel),
		fmt.Sprintf("-XX:FreqInlineSize=%d", cfg.FreqInlineSize),

		"-Djdk.nio.maxCachedBufferSize=131072",
	}

	if cfg.PreTouch {
		flags = append(flags, "-XX:+AlwaysPreTouch")
	}
	if cfg.G1SATBBufferEnqueueingThresholdPercent > 0 {
		flags = append(flags, fmt.Sprintf("-XX:G1SATBBufferEnqueueingThresholdPercent=%d", cfg.G1SATBBufferEnqueueingThresholdPercent))
	}
	if cfg.G1ConcRSHotCardLimit > 0 {
		flags = append(flags, fmt.Sprintf("-XX:G1ConcRSHotCardLimit=%d", cfg.G1ConcRSHotCardLimit))
	}
	if cfg.G1ConcRefinementServiceIntervalMillis > 0 {
		flags = append(flags, fmt.Sprintf("-XX:G1ConcRefinementServiceIntervalMillis=%d", cfg.G1ConcRefinementServiceIntervalMillis))
	}
	if cfg.GCTimeRatio > 0 {
		flags = append(flags, fmt.Sprintf("-XX:GCTimeRatio=%d", cfg.GCTimeRatio))
	}
	if cfg.UseDynamicNumberOfGCThreads {
		flags = append(flags, "-XX:+UseDynamicNumberOfGCThreads")
	}
	if cfg.UseStringDeduplication {
		flags = append(flags, "-XX:+UseStringDeduplication")
	}
	if cfg.InlineSmallCode > 0 {
		flags = append(flags, fmt.Sprintf("-XX:InlineSmallCode=%d", cfg.InlineSmallCode))
	}
	if cfg.MaxNodeLimit > 0 && cfg.NodeLimitFudgeFactor > 0 {
		flags = append(flags, fmt.Sprintf("-XX:NodeLimitFudgeFactor=%d", cfg.NodeLimitFudgeFactor))
		flags = append(flags, fmt.Sprintf("-XX:MaxNodeLimit=%d", cfg.MaxNodeLimit))
	}
	if cfg.NmethodSweepActivity > 0 {
		flags = append(flags, fmt.Sprintf("-XX:NmethodSweepActivity=%d", cfg.NmethodSweepActivity))
	}
	if !cfg.DontCompileHugeMethods {
		flags = append(flags, "-XX:-DontCompileHugeMethods")
	}
	if cfg.AllocatePrefetchStyle > 0 {
		flags = append(flags, fmt.Sprintf("-XX:AllocatePrefetchStyle=%d", cfg.AllocatePrefetchStyle))
	}
	if cfg.AlwaysActAsServerClass {
		flags = append(flags, "-XX:+AlwaysActAsServerClassMachine")
	}
	if cfg.UseXMMForArrayCopy {
		flags = append(flags, "-XX:+UseXMMForArrayCopy")
	}
	if cfg.UseFPUForSpilling {
		flags = append(flags, "-XX:+UseFPUForSpilling")
	}
	if cfg.UseLargePages {
		flags = append(flags, "-XX:+UseLargePages")
	}

	return flags
}
