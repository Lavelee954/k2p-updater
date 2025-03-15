package server

import (
	"context"
	"fmt"
	"k2p-updater/cmd/app"
	"k2p-updater/internal/features/exporter"
	"k2p-updater/internal/features/exporter/domain"
	"k2p-updater/pkg/resource"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Run starts the application
func Run() {
	// 컨텍스트 생성 및 신호 처리 설정
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 신호 처리를 위한 채널 생성
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	// 백그라운드에서 신호 감지
	go func() {
		sig := <-signals
		log.Printf("시그널 수신: %v, 종료 시작", sig)
		cancel()
	}()

	// 1. 설정 로드
	cfg, err := app.Load()
	if err != nil {
		log.Fatalf("설정 로드 실패: %v", err)
	}

	// 2. Kubernetes 클라이언트 생성
	kcfg, err := app.NewKubeClients(&cfg.Kubernetes)
	if err != nil {
		log.Fatalf("Kubernetes 클라이언트 생성 실패: %v", err)
	}

	// 3. 익스포터 서비스 초기화 및 시작
	exporterProvider, err := initializeExporterService(ctx, cfg, kcfg.ClientSet)
	if err != nil {
		log.Fatalf("익스포터 서비스 초기화 실패: %v", err)
	}
	log.Println("익스포터 서비스 성공적으로 시작됨")

	// 4. VM 상태 확인기 생성
	vmVerifier := exporter.NewVMHealthVerifier(
		kcfg.ClientSet,
		exporterProvider,
		&cfg.Exporter,
	)

	// 5. 리소스 정의 변환
	resourceDefs := convertResourceDefinitions(cfg.Resources.Definitions)

	// 6. 팩토리 생성
	factory, err := resource.NewFactory(
		cfg.Resources.Namespace,
		cfg.Resources.Group,
		cfg.Resources.Version,
		resourceDefs,
		kcfg.DynamicClient,
		kcfg.ClientSet,
	)
	if err != nil {
		log.Fatalf("리소스 팩토리 생성 실패: %v", err)
		return
	}

	// 7. 애플리케이션 실행
	if err := runApplication(ctx, factory, exporterProvider, vmVerifier); err != nil {
		log.Printf("애플리케이션 오류: %v", err)
	}

	// 정상 종료 메시지
	log.Println("애플리케이션 종료 완료")
}

// initializeExporterService initializes and starts the exporter service
func initializeExporterService(ctx context.Context, cfg *app.Config, client app.KubeClientInterface) (domain.Provider, error) {
	// 초기화 컨텍스트 생성 (타임아웃 설정)
	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 익스포터 서비스 생성 및 시작
	log.Println("익스포터 서비스 초기화 중...")
	provider, err := exporter.NewProvider(initCtx, &cfg.Exporter, client)
	if err != nil {
		return nil, fmt.Errorf("익스포터 제공자 생성 실패: %w", err)
	}

	// 서비스 초기화 완료 대기
	if err := provider.WaitForInitialization(initCtx); err != nil {
		return nil, fmt.Errorf("익스포터 서비스 초기화 시간 초과: %w", err)
	}

	log.Printf("익스포터 서비스 초기화 완료 - %d개의 정상 익스포터 발견",
		len(provider.GetHealthyExporters()))

	return provider, nil
}

// runApplication contains the main application logic
func runApplication(
	ctx context.Context,
	factory *resource.Factory,
	exporterProvider domain.Provider,
	vmVerifier domain.VMHealthVerifier,
) error {
	// 이벤트 기록
	statusMsg := "정상적으로 등록되었습니다."
	if err := factory.Event().NormalRecord(ctx, "updater", "VmSpecUp", statusMsg); err != nil {
		return fmt.Errorf("이벤트 기록 실패: %w", err)
	}

	// 상태 업데이트 준비
	statusData := map[string]interface{}{
		"controlPlaneNodeName": "cp-k8s",
		"message":              statusMsg,
		"lastUpdateTime":       time.Now().Format(time.RFC3339),
		"coolDown":             true,
		"updateStatus":         "Pending",
	}

	// 익스포터 상태 정보 추가
	healthyExporters := exporterProvider.GetHealthyExporters()
	statusData["healthyExporterCount"] = len(healthyExporters)

	if len(healthyExporters) > 0 {
		// 예시: 첫 번째 익스포터의 상태 확인
		nodeToCheck := healthyExporters[0].NodeName
		upgradeTime := time.Now().Add(-10 * time.Minute) // 가정: 10분 전에 업그레이드됨

		healthy, err := vmVerifier.IsVMHealthy(ctx, nodeToCheck, upgradeTime)
		if err != nil {
			log.Printf("%s의 VM 건강 확인 실패: %v", nodeToCheck, err)
		} else {
			statusData["vmHealthy"] = healthy
			log.Printf("VM %s 건강 상태: %v", nodeToCheck, healthy)
		}
	}

	// 상태 업데이트
	if err := factory.Status().UpdateGeneric(ctx, "updater", statusData); err != nil {
		return fmt.Errorf("상태 업데이트 실패: %w", err)
	}

	// 메인 애플리케이션 루프 실행
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	log.Println("애플리케이션 실행 중, 종료하려면 Ctrl+C를 누르세요")

	for {
		select {
		case <-ctx.Done():
			log.Println("애플리케이션 컨텍스트 취소됨, 종료 중...")
			return nil
		case <-ticker.C:
			// 주기적인 상태 업데이트 수행
			healthyCount := len(exporterProvider.GetHealthyExporters())
			log.Printf("상태 확인: %d개의 정상 익스포터", healthyCount)

			// 상태 업데이트 수행
			updateData := map[string]interface{}{
				"lastCheckTime":        time.Now().Format(time.RFC3339),
				"healthyExporterCount": healthyCount,
				"updateStatus":         "Running",
			}

			if err := factory.Status().UpdateGeneric(ctx, "updater", updateData); err != nil {
				log.Printf("상태 업데이트 실패: %v", err)
			}
		}
	}
}

// convertResourceDefinitions converts app-specific configuration to resource package format
func convertResourceDefinitions(appDefs map[string]app.ResourceDefinitionConfig) map[string]resource.FactoryDefinition {
	resourceDefs := make(map[string]resource.FactoryDefinition)

	for key, appDef := range appDefs {
		resourceDefs[key] = resource.FactoryDefinition{
			Resource:    appDef.Resource,
			NameFormat:  appDef.NameFormat,
			StatusField: appDef.StatusField,
			Kind:        appDef.Kind,
			CRName:      appDef.CRName,
		}
	}

	return resourceDefs
}
