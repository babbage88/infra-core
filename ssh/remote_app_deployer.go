package ssh

import (
	"fmt"
	"log/slog"

	"github.com/babbage88/goph/v2"
	"github.com/pkg/sftp"
)

func NewRemoteAppDeploymentAgentWithPassword(hostname, sshUser, srcUtilsPath, dstUtilsPath, sshPassword string, envVars map[string]string, port uint) (*RemoteAppDeploymentAgent, error) {
	sshClient, err := goph.NewConn(&goph.Config{
		User:     sshUser,
		Addr:     hostname,
		Port:     port,
		Auth:     goph.Password(sshPassword),
		Callback: VerifyHost,
	})
	if err != nil {
		slog.Error("error initializing ssh client", slog.String("error", err.Error()))
		return nil, SshErrorWrapper(500, err, "failed to initialize ssh client")
	}
	return &RemoteAppDeploymentAgent{
		SshClient:           sshClient,
		SourceUtilsDir:      srcUtilsPath,
		DestinationUtilsDir: dstUtilsPath,
		EnvVars:             envVars,
	}, nil
}

func NewRemoteAppDeploymentAgentWithSshKey(hostname, sshUser, srcUtilsPath, dstUtilsPath, sshKey, sshPassphrase string, envVars map[string]string, agent bool, port uint) (*RemoteAppDeploymentAgent, error) {
	sshClient, err := initializeSshClient(hostname, sshUser, port, sshKey, sshPassphrase, agent)
	if err != nil {
		slog.Error("Error initializing ssh client", "error", err.Error())
		return nil, SshErrorWrapper(500, err, "failed to initialize ssh client")
	}
	return &RemoteAppDeploymentAgent{
		SshClient:           sshClient,
		SourceUtilsDir:      srcUtilsPath,
		DestinationUtilsDir: dstUtilsPath,
		EnvVars:             envVars,
	}, nil
}

func InitializeRemoteSshAgent(hostname, sshUser, sshKey, sshPassphrase string, agent bool, port uint) (*RemoteAppDeploymentAgent, error) {
	sshClient, err := initializeSshClient(hostname, sshUser, port, sshKey, sshPassphrase, agent)
	if err != nil {
		slog.Error("Error initializing ssh client", "error", err.Error())
		return nil, SshErrorWrapper(500, err, "failed to initialize ssh client")
	}
	return &RemoteAppDeploymentAgent{SshClient: sshClient}, nil
}

func (r *RemoteAppDeploymentAgent) CopyUtilsToRemoteHost() error {
	if err := r.SshClient.Upload(r.SourceUtilsDir, r.DestinationUtilsDir); err != nil {
		slog.Error("Error uploading RemoteUtils", slog.String("src", r.SourceUtilsDir), slog.String("dst", r.DestinationUtilsDir), "error", err.Error())
		return SftpErrorWrapper(501, err, "error preforming upload over sftp")
	}
	return nil
}

func (r *RemoteAppDeploymentAgent) Upload(src, dst string) error {
	if err := r.SshClient.Upload(src, dst); err != nil {
		slog.Error("Error uploading files to remote", slog.String("src", src), slog.String("dst", dst), "error", err.Error())
		return SftpErrorWrapper(501, err, "error preforming upload over sftp")
	}
	return nil
}

func (r *RemoteAppDeploymentAgent) UploadBin(src, dst string) error {
	if err := r.SshClient.Upload(src, dst); err != nil {
		slog.Error("Error uploading files to remote", slog.String("src", src), slog.String("dst", dst), "error", err.Error())
		return SftpErrorWrapper(501, err, "error preforming upload over sftp")
	}
	r.RunCommand("chmod", []string{"+x", dst})
	return nil
}

func (r *RemoteAppDeploymentAgent) Download(src, dst string) error {
	if err := r.SshClient.Download(src, dst); err != nil {
		slog.Error("Error download files from remote  dst: %s err: %s\n", slog.String("src", src), slog.String("dst", dst), "error", err.Error())
		return SftpErrorWrapper(501, err, "error preforming upload over sftp")
	}
	return nil
}

func (r *RemoteAppDeploymentAgent) GetSftpClient() (*sftp.Client, error) {
	sftpClient, err := r.SshClient.NewSftp()
	if err != nil {
		slog.Error("Error initializing sftp client", "error", err.Error())
		return nil, SftpInitErrorWrapper(503, err, "error preforming upload over sftp")
	}
	return sftpClient, nil
}

func (r *RemoteAppDeploymentAgent) WriteBytesSftp(destinationPath string, data []byte) (int, error) {
	sftpClient, err := r.GetSftpClient()
	if err != nil {
		slog.Error("Error initializing sftp client", "error", err.Error())
		return 0, SftpInitErrorWrapper(503, err, "error preforming upload over sftp")
	}
	slog.Info("Creating sftp client on remote host", slog.String("destinationPath", destinationPath))
	f, err := sftpClient.Create(destinationPath)
	if err != nil {
		return 0, SftpFileCreationErrorWrapper(504, err, "error creating file via sftp client")
	}
	bytesWritten, err := f.Write(data)
	defer f.Close()
	if err != nil {
		return 0, SftpFileCreationErrorWrapper(504, err, "error creating file via sftp client")
	}
	slog.Info("Finished writing file: %s bytes: %d remote host", slog.String("file", destinationPath), slog.Int("bytesWritten", bytesWritten))
	return bytesWritten, nil
}

func (r *RemoteAppDeploymentAgent) RunCommand(remoteCmd string, args []string) error {
	cmd, err := r.SshClient.Command(remoteCmd, args...)
	cmd.Env = r.GetEnvarSlice()
	slog.Info("Executing remote command", slog.String("cmd", remoteCmd), slog.Any("args", args))
	if err != nil {
		slog.Error("error initializing goph Command", "error", err.Error())
		return err
	}
	err = cmd.Run()
	if err != nil {
		slog.Error("error Running goph Command", "error", err.Error())
	}
	return err
}

func (r *RemoteAppDeploymentAgent) RunCommandAndCaptureOutput(remoteCmd string, args []string) ([]byte, error) {
	cmd, err := r.SshClient.Command(remoteCmd, args...)
	if err != nil {
		return nil, err
	}
	cmd.Env = r.GetEnvarSlice()
	combinedOutput, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Error:", err)
		return combinedOutput, err
	}
	return combinedOutput, err
}

func (r *RemoteAppDeploymentAgent) GetEnvarSlice() []string {
	argEnvars := make([]string, 0, len(r.EnvVars))
	for k, v := range r.EnvVars {
		argEnvars = append(argEnvars, fmt.Sprintf("%s=%s", k, v))
	}
	return argEnvars
}
