#!/usr/bin/env bash
# GCP GKE Test Setup Script
# Kullanım: ./scripts/gcp-setup.sh [setup|build|deploy|teardown]

set -euo pipefail

# ── Değişkenler ──────────────────────────────────────────────
PROJECT_ID="${GCP_PROJECT_ID:-optimizer-demo}"
REGION="${GCP_REGION:-us-central1}"
ZONE="${GCP_ZONE:-us-central1-a}"
CLUSTER_NAME="${CLUSTER_NAME:-optimizer-test-cluster}"
REPO_NAME="${REPO_NAME:-optimizer}"
IMAGE_NAME="intelligent-cluster-optimizer"
IMAGE_TAG="${IMAGE_TAG:-latest}"
NAMESPACE="intelligent-optimizer-system"

REGISTRY="${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPO_NAME}"
FULL_IMAGE="${REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}"

# ── Renkli çıktı ─────────────────────────────────────────────
GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
info()    { echo -e "${GREEN}[INFO]${NC} $*"; }
warning() { echo -e "${YELLOW}[WARN]${NC} $*"; }
error()   { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }

# ── Komut kontrolü ───────────────────────────────────────────
check_tools() {
    for tool in gcloud kubectl docker; do
        command -v "$tool" &>/dev/null || error "$tool kurulu değil"
    done
    info "Tüm araçlar mevcut."
}

# ── GCP Hazırlık ─────────────────────────────────────────────
setup_gcp() {
    info "GCP projesi ayarlanıyor: ${PROJECT_ID}"
    gcloud config set project "${PROJECT_ID}"

    info "Gerekli API'ler etkinleştiriliyor..."
    gcloud services enable \
        container.googleapis.com \
        artifactregistry.googleapis.com \
        --quiet

    info "Artifact Registry repo oluşturuluyor: ${REPO_NAME}"
    gcloud artifacts repositories create "${REPO_NAME}" \
        --repository-format=docker \
        --location="${REGION}" \
        --description="Intelligent Cluster Optimizer images" \
        --quiet 2>/dev/null || warning "Repo zaten mevcut, devam ediliyor."

    info "Docker auth yapılandırılıyor..."
    gcloud auth configure-docker "${REGION}-docker.pkg.dev" --quiet

    info "GKE cluster oluşturuluyor: ${CLUSTER_NAME}"
    info "  - Makine: e2-standard-2 (2 vCPU, 8GB)"
    info "  - Node sayısı: 2"
    info "  - Bölge: ${ZONE}"
    gcloud container clusters create "${CLUSTER_NAME}" \
        --zone="${ZONE}" \
        --machine-type="e2-standard-2" \
        --num-nodes=2 \
        --enable-autoscaling \
        --min-nodes=1 \
        --max-nodes=4 \
        --enable-autorepair \
        --enable-autoupgrade \
        --workload-pool="${PROJECT_ID}.svc.id.goog" \
        --quiet

    info "kubectl credentials alınıyor..."
    gcloud container clusters get-credentials "${CLUSTER_NAME}" \
        --zone="${ZONE}" \
        --project="${PROJECT_ID}"

    info "Metrics Server etkinleştiriliyor..."
    # GKE'de Metrics Server genellikle otomatik gelir, kontrol edelim
    kubectl wait --for=condition=available --timeout=60s \
        deployment/metrics-server -n kube-system 2>/dev/null \
        || warning "Metrics Server hazır değil, birkaç dakika bekleyin."

    info "✅ GCP kurulumu tamamlandı!"
    echo ""
    echo "Registry: ${REGISTRY}"
    echo "Cluster:  ${CLUSTER_NAME}"
}

# ── Docker Build & Push ──────────────────────────────────────
build_and_push() {
    info "Docker image build ediliyor: ${FULL_IMAGE}"
    docker build \
        --platform=linux/amd64 \
        -t "${FULL_IMAGE}" \
        -t "${REGISTRY}/${IMAGE_NAME}:$(git rev-parse --short HEAD 2>/dev/null || echo dev)" \
        .

    info "Image push ediliyor..."
    docker push "${FULL_IMAGE}"
    docker push "${REGISTRY}/${IMAGE_NAME}:$(git rev-parse --short HEAD 2>/dev/null || echo dev)"

    info "✅ Image hazır: ${FULL_IMAGE}"
}

# ── Kubernetes Deploy ─────────────────────────────────────────
deploy() {
    info "kubectl bağlantısı kontrol ediliyor..."
    kubectl cluster-info || error "kubectl cluster'a bağlanamıyor"

    info "deployment.yaml image güncelleniyor: ${FULL_IMAGE}"
    # Geçici manifest oluştur (orijinali değiştirme)
    local tmp_dir
    tmp_dir=$(mktemp -d)
    cp -r deploy/* "${tmp_dir}/"

    # Image'ı güncelle
    sed -i.bak "s|intelligent-cluster-optimizer:latest|${FULL_IMAGE}|g" \
        "${tmp_dir}/deployment.yaml"
    sed -i.bak "s|imagePullPolicy: IfNotPresent|imagePullPolicy: Always|g" \
        "${tmp_dir}/deployment.yaml"

    info "Kubernetes manifests deploy ediliyor..."
    kubectl apply -f "${tmp_dir}/namespace.yaml"
    kubectl apply -f "${tmp_dir}/serviceaccount.yaml"
    kubectl apply -f "${tmp_dir}/rbac.yaml"
    kubectl apply -f "${tmp_dir}/deployment.yaml"
    kubectl apply -f "${tmp_dir}/service.yaml"

    rm -rf "${tmp_dir}"

    info "CRD yükleniyor..."
    kubectl apply -f config/crd/ 2>/dev/null || warning "CRD dizini bulunamadı, atlıyorum."

    info "Deployment bekleniyor..."
    kubectl rollout status deployment/intelligent-optimizer-controller \
        -n "${NAMESPACE}" --timeout=120s

    info "✅ Deploy tamamlandı!"
    echo ""
    kubectl get pods -n "${NAMESPACE}"
    echo ""
    echo "Logları izlemek için:"
    echo "  kubectl logs -n ${NAMESPACE} -l app.kubernetes.io/component=controller -f"
    echo ""
    echo "Metrics için port-forward:"
    echo "  kubectl port-forward -n ${NAMESPACE} svc/intelligent-optimizer-metrics 8080:8080"
}

# ── Status ────────────────────────────────────────────────────
status() {
    echo ""
    info "=== Cluster Durumu ==="
    kubectl get nodes
    echo ""
    info "=== Optimizer Pods ==="
    kubectl get pods -n "${NAMESPACE}" -o wide
    echo ""
    info "=== Son Loglar ==="
    kubectl logs -n "${NAMESPACE}" \
        -l app.kubernetes.io/component=controller \
        --tail=30 2>/dev/null || warning "Pod henüz başlamadı"
}

# ── Teardown ──────────────────────────────────────────────────
teardown() {
    warning "Bu işlem GKE cluster'ı SİLECEK: ${CLUSTER_NAME}"
    read -r -p "Devam etmek istiyor musunuz? (yes/no): " confirm
    [[ "${confirm}" == "yes" ]] || { info "İptal edildi."; exit 0; }

    info "Kubernetes kaynakları siliniyor..."
    kubectl delete namespace "${NAMESPACE}" --ignore-not-found

    info "GKE cluster siliniyor: ${CLUSTER_NAME}"
    gcloud container clusters delete "${CLUSTER_NAME}" \
        --zone="${ZONE}" \
        --quiet

    info "✅ Teardown tamamlandı."
}

# ── Main ──────────────────────────────────────────────────────
main() {
    check_tools

    local cmd="${1:-help}"
    case "${cmd}" in
        setup)     setup_gcp ;;
        build)     build_and_push ;;
        deploy)    deploy ;;
        all)       setup_gcp && build_and_push && deploy ;;
        status)    status ;;
        teardown)  teardown ;;
        *)
            echo "Kullanım: $0 {setup|build|deploy|all|status|teardown}"
            echo ""
            echo "  setup     - GCP API, Artifact Registry ve GKE cluster oluştur"
            echo "  build     - Docker image build et ve push et"
            echo "  deploy    - Kubernetes manifests'leri cluster'a uygula"
            echo "  all       - setup + build + deploy (tam kurulum)"
            echo "  status    - Cluster ve pod durumunu göster"
            echo "  teardown  - Tüm GCP kaynaklarını sil"
            ;;
    esac
}

main "$@"
