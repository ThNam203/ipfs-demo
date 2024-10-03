package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/ipfs/boxo/files"
	"github.com/ipfs/boxo/path"
	"github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core"
	"github.com/ipfs/kubo/core/coreapi"
	"github.com/ipfs/kubo/core/corehttp"
	iface "github.com/ipfs/kubo/core/coreiface"
	"github.com/ipfs/kubo/core/coreiface/options"
	"github.com/ipfs/kubo/core/node/libp2p"
	"github.com/ipfs/kubo/plugin/loader"
	"github.com/ipfs/kubo/repo/fsrepo"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

type IPFSStorage struct {
	ipfsApi iface.CoreAPI
	node    *core.IpfsNode
	ctx     context.Context
}

func NewIPFSStorage(ctx context.Context) *IPFSStorage {
	ipfsStorage := &IPFSStorage{
		ctx: ctx,
	}

	ipfsStorage.setup()

	return ipfsStorage
}

func (s *IPFSStorage) setup() {
	ipfsApi, node, err := s.spawnEphemeral()
	if err != nil {
		log.Panic(err)
	}

	s.ipfsApi = *ipfsApi
	s.node = node

	defer log.Println("IPFS node exited")
	log.Println("IPFS node is running")
	fmt.Printf("ipfs addresses: %v\n", s.node.PeerHost.Addrs())

	go s.goOnlineIPFSNode()
}

func (s *IPFSStorage) Save(filePath string) (path.Path, error) {
	file, err := getUnixfsNode(filePath)
	defer file.Close()

	if err != nil {
		return nil, fmt.Errorf("failed to get file: %s", err)
	}

	opts := []options.UnixfsAddOption{
		options.Unixfs.Pin(false),
	}

	fileCid, err := s.ipfsApi.Unixfs().Add(s.ctx, file, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to add file into ipfs: %s", err)
	}

	return fileCid, err
}

func getUnixfsNode(path string) (files.Node, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	f, err := files.NewSerialFile(path, false, st)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func createIPFSNode(ctx context.Context, repoPath string) (*iface.CoreAPI, *core.IpfsNode, error) {
	repo, err := fsrepo.Open(repoPath)
	if err != nil {
		return nil, nil, err
	}

	repo.SetConfigKey("Addresses.Gateway", config.Strings{"/ip4/0.0.0.0/tcp/8080"})

	nodeOptions := &core.BuildCfg{
		Online:  true,
		Routing: libp2p.DHTOption,
		Repo:    repo,
	}

	node, err := core.NewNode(ctx, nodeOptions)
	node.IsDaemon = true

	if err != nil {
		return nil, nil, err
	}

	coreApi, err := coreapi.NewCoreAPI(node)
	return &coreApi, node, nil
}

// Must load plugins before setting up everything
func setupPlugins(repoPath string) error {
	plugins, err := loader.NewPluginLoader(repoPath)
	if err != nil {
		return fmt.Errorf("error loading plugins: %s", err)
	}

	if err := plugins.Initialize(); err != nil {
		return fmt.Errorf("error initializing plugins: %s", err)
	}

	if err := plugins.Inject(); err != nil {
		return fmt.Errorf("error initializing plugins: %s", err)
	}

	return nil
}

func createTempRepo() (string, error) {
	repoPath, err := os.MkdirTemp("", "ipfs-shell")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir for ipfs: %s", err)
	}

	cfg, err := config.Init(log.Writer(), 2048)
	if err != nil {
		return "", fmt.Errorf("failed to init config file for repo: %s", err)
	}

	// https://github.com/ipfs/kubo/blob/master/docs/experimental-features.md#ipfs-filestore
	cfg.Experimental.FilestoreEnabled = true
	// https://github.com/ipfs/kubo/blob/master/docs/experimental-features.md#ipfs-urlstore
	cfg.Experimental.UrlstoreEnabled = true
	// https://github.com/ipfs/kubo/blob/master/docs/experimental-features.md#ipfs-p2p
	cfg.Experimental.Libp2pStreamMounting = true
	// https://github.com/ipfs/kubo/blob/master/docs/experimental-features.md#p2p-http-proxy
	cfg.Experimental.P2pHttpProxy = true
	// See also: https://github.com/ipfs/kubo/blob/master/docs/config.md
	// And: https://github.com/ipfs/kubo/blob/master/docs/experimental-features.md

	err = fsrepo.Init(repoPath, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to create ephemeral node: %s", err)
	}
	return repoPath, nil
}

var loadPluginsOnce sync.Once

// Function "spawnEphemeral" Create a temporary just for one run
func (s *IPFSStorage) spawnEphemeral() (*iface.CoreAPI, *core.IpfsNode, error) {
	var onceErr error
	loadPluginsOnce.Do(func() {
		onceErr = setupPlugins("")
	})

	if onceErr != nil {
		return nil, nil, onceErr
	}

	// Create a Temporary Repo
	repoPath, err := createTempRepo()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temp repo: %s", err)
	}

	api, node, err := createIPFSNode(s.ctx, repoPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create ipfs node: %s", err)
	}

	return api, node, err
}

func (s *IPFSStorage) connectToPeers(peers []string) error {
	var wg sync.WaitGroup
	peerInfos := make(map[peer.ID]*peer.AddrInfo, len(peers))
	for _, addrStr := range peers {
		addr, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			return err
		}
		pii, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			return err
		}
		pi, ok := peerInfos[pii.ID]
		if !ok {
			pi = &peer.AddrInfo{ID: pii.ID}
			peerInfos[pi.ID] = pi
		}
		pi.Addrs = append(pi.Addrs, pii.Addrs...)
	}

	wg.Add(len(peerInfos))
	for _, peerInfo := range peerInfos {
		go func(peerInfo *peer.AddrInfo) {
			defer wg.Done()
			err := s.ipfsApi.Swarm().Connect(s.ctx, *peerInfo)
			if err != nil {
				log.Printf("failed to connect to %s: %s", peerInfo.ID, err)
			} else {
				log.Printf("ipfs connectted to %s", peerInfo.ID)
			}
		}(peerInfo)
	}
	wg.Wait()
	return nil
}

func (s *IPFSStorage) goOnlineIPFSNode() {
	bootstrapNodes := []string{
		// IPFS Bootstrapper nodes.
		"/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
		"/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
		"/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
		"/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
		"/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
	}

	go s.connectToPeers(bootstrapNodes)

	addr := "/ip4/0.0.0.0/tcp/8080"
	var opts = []corehttp.ServeOption{
		corehttp.GatewayOption("/ipfs", "/ipns"),
	}

	if err := corehttp.ListenAndServe(s.node, addr, opts...); err != nil {
		log.Printf("ipfs bootstraping failed: %s", err)
		return
	}
}
