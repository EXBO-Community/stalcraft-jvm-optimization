package main

import "strings"

var exactRemove = map[string]bool{
	"-XX:-PrintCommandLineFlags":        true,
	"-XX:+UseG1GC":                      true,
	"-XX:+UseCompressedOops":            true,
	"-XX:+PerfDisableSharedMem":         true,
	"-XX:+UseBiasedLocking":             true,
	"-XX:-UseBiasedLocking":             true,
	"-XX:+UseStringDeduplication":        true,
	"-XX:+UseNUMA":                      true,
	"-XX:+DisableAttachMechanism":        true,
	"-XX:+UseDynamicNumberOfGCThreads":   true,
	"-XX:+AlwaysActAsServerClassMachine": true,
	"-XX:+UseXMMForArrayCopy":           true,
	"-XX:+UseFPUForSpilling":            true,
	"-XX:-DontCompileHugeMethods":       true,
	"-XX:+AlwaysPreTouch":               true,
	"-XX:+ParallelRefProcEnabled":        true,
	"-XX:+DisableExplicitGC":            true,
	"-XX:+G1UseAdaptiveIHOP":            true,
	"-XX:+UnlockExperimentalVMOptions":  true,
}

var prefixRemove = []string{
	"-XX:MaxGCPauseMillis=",
	"-XX:MetaspaceSize=",
	"-XX:MaxMetaspaceSize=",
	"-XX:G1HeapRegionSize=",
	"-XX:G1NewSizePercent=",
	"-XX:G1MaxNewSizePercent=",
	"-XX:G1ReservePercent=",
	"-XX:G1HeapWastePercent=",
	"-XX:G1MixedGCCountTarget=",
	"-XX:InitiatingHeapOccupancyPercent=",
	"-XX:G1MixedGCLiveThresholdPercent=",
	"-XX:G1RSetUpdatingPauseTimePercent=",
	"-XX:G1SATBBufferEnqueueingThresholdPercent=",
	"-XX:G1ConcRSHotCardLimit=",
	"-XX:G1ConcRefinementServiceIntervalMillis=",
	"-XX:GCTimeRatio=",
	"-XX:SurvivorRatio=",
	"-XX:MaxTenuringThreshold=",
	"-XX:ParallelGCThreads=",
	"-XX:ConcGCThreads=",
	"-XX:SoftRefLRUPolicyMSPerMB=",
	"-XX:ReservedCodeCacheSize=",
	"-XX:NonNMethodCodeHeapSize=",
	"-XX:ProfiledCodeHeapSize=",
	"-XX:NonProfiledCodeHeapSize=",
	"-XX:MaxInlineLevel=",
	"-XX:FreqInlineSize=",
	"-XX:InlineSmallCode=",
	"-XX:MaxNodeLimit=",
	"-XX:NodeLimitFudgeFactor=",
	"-XX:NmethodSweepActivity=",
	"-XX:AllocatePrefetchStyle=",
	"-XX:LargePageSizeInBytes=",
	"-Xms",
	"-Xmx",
}

func splitArgs(args []string) (jvm []string, mainClass string, app []string) {
	for i := 0; i < len(args); {
		a := args[i]
		if a == "-classpath" || a == "-cp" || a == "-jar" {
			jvm = append(jvm, a)
			i++
			if i < len(args) {
				jvm = append(jvm, args[i])
			}
			i++
			continue
		}
		if strings.HasPrefix(a, "-") {
			jvm = append(jvm, a)
			i++
			continue
		}
		mainClass = a
		app = args[i+1:]
		return
	}
	return
}

func shouldRemove(arg string) bool {
	if exactRemove[arg] {
		return true
	}
	for _, p := range prefixRemove {
		if strings.HasPrefix(arg, p) {
			return true
		}
	}
	return false
}

func filterArgs(orig, injected []string) []string {
	jvm, mainClass, app := splitArgs(orig)

	var filtered []string
	for _, a := range jvm {
		if !shouldRemove(a) {
			filtered = append(filtered, a)
		}
	}
	result := make([]string, 0, len(filtered)+len(injected)+1+len(app))
	result = append(result, filtered...)
	result = append(result, injected...)
	if mainClass != "" {
		result = append(result, mainClass)
	}
	return append(result, app...)
}
