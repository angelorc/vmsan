/* eslint-disable */
/* prettier-ignore */
/**
 * This file was auto-generated from the Firecracker API Swagger spec.
 * Source: https://raw.githubusercontent.com/firecracker-microvm/firecracker/v1.14.1/src/firecracker/swagger/firecracker.yaml
 * Version: v1.14.1
 * Generated: 2026-02-27T13:40:26.678Z
 *
 * DO NOT EDIT — regenerate with: pnpm generate:firecracker
 */
export interface paths {
    "/": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** Returns general information about an instance. */
        get: operations["describeInstance"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/actions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /** Creates a synchronous action. */
        put: operations["createSyncAction"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/balloon": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** Returns the current balloon device configuration. */
        get: operations["describeBalloonConfig"];
        /**
         * Creates or updates a balloon device.
         * @description Creates a new balloon device if one does not already exist, otherwise updates it, before machine startup. This will fail after machine startup. Will fail if update is not possible.
         */
        put: operations["putBalloon"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /**
         * Updates a balloon device.
         * @description Updates an existing balloon device, before or after machine startup. Will fail if update is not possible.
         */
        patch: operations["patchBalloon"];
        trace?: never;
    };
    "/balloon/statistics": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** Returns the latest balloon device statistics, only if enabled pre-boot. */
        get: operations["describeBalloonStats"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /**
         * Updates a balloon device statistics polling interval.
         * @description Updates an existing balloon device statistics interval, before or after machine startup. Will fail if update is not possible.
         */
        patch: operations["patchBalloonStatsInterval"];
        trace?: never;
    };
    "/balloon/hinting/start": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /** Starts a free page hinting run only if enabled pre-boot. */
        patch: operations["startBalloonHinting"];
        trace?: never;
    };
    "/balloon/hinting/status": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** Returns the balloon hinting statistics, only if enabled pre-boot. */
        get: operations["describeBalloonHinting"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/balloon/hinting/stop": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /** Stops a free page hinting run only if enabled pre-boot. */
        patch: operations["stopBalloonHinting"];
        trace?: never;
    };
    "/boot-source": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /**
         * Creates or updates the boot source. Pre-boot only.
         * @description Creates new boot source if one does not already exist, otherwise updates it. Will fail if update is not possible.
         */
        put: operations["putGuestBootSource"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/cpu-config": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /**
         * Configures CPU features flags for the vCPUs of the guest VM. Pre-boot only.
         * @description Provides configuration to the Firecracker process to specify vCPU resource configuration prior to launching the guest machine.
         */
        put: operations["putCpuConfiguration"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/drives/{drive_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /**
         * Creates or updates a drive. Pre-boot only.
         * @description Creates new drive with ID specified by drive_id path parameter. If a drive with the specified ID already exists, updates its state based on new input. Will fail if update is not possible.
         */
        put: operations["putGuestDriveByID"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /**
         * Updates the properties of a drive. Post-boot only.
         * @description Updates the properties of the drive with the ID specified by drive_id path parameter. Will fail if update is not possible.
         */
        patch: operations["patchGuestDriveByID"];
        trace?: never;
    };
    "/pmem/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /**
         * Creates or updates a pmem device. Pre-boot only.
         * @description Creates new pmem device with ID specified by id parameter. If a pmem device with the specified ID already exists, updates its state based on new input. Will fail if update is not possible.
         */
        put: operations["putGuestPmemByID"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/logger": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /** Initializes the logger by specifying a named pipe or a file for the logs output. */
        put: operations["putLogger"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/machine-config": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Gets the machine configuration of the VM.
         * @description Gets the machine configuration of the VM. When called before the PUT operation, it will return the default values for the vCPU count (=1), memory size (=128 MiB). By default SMT is disabled and there is no CPU Template.
         */
        get: operations["getMachineConfiguration"];
        /**
         * Updates the Machine Configuration of the VM. Pre-boot only.
         * @description Updates the Virtual Machine Configuration with the specified input. Firecracker starts with default values for vCPU count (=1) and memory size (=128 MiB). The vCPU count is restricted to the [1, 32] range. With SMT enabled, the vCPU count is required to be either 1 or an even number in the range. otherwise there are no restrictions regarding the vCPU count. If 2M hugetlbfs pages are specified, then `mem_size_mib` must be a multiple of 2. If any of the parameters has an incorrect value, the whole update fails. All parameters that are optional and are not specified are set to their default values (smt = false, track_dirty_pages = false, cpu_template = None, huge_pages = None).
         */
        put: operations["putMachineConfiguration"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /**
         * Partially updates the Machine Configuration of the VM. Pre-boot only.
         * @description Partially updates the Virtual Machine Configuration with the specified input. If any of the parameters has an incorrect value, the whole update fails.
         */
        patch: operations["patchMachineConfiguration"];
        trace?: never;
    };
    "/metrics": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /** Initializes the metrics system by specifying a named pipe or a file for the metrics output. */
        put: operations["putMetrics"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/mmds": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** Get the MMDS data store. */
        get: operations["getMmds"];
        /** Creates a MMDS (Microvm Metadata Service) data store. */
        put: operations["putMmds"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /** Updates the MMDS data store. */
        patch: operations["patchMmds"];
        trace?: never;
    };
    "/mmds/config": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /**
         * Set MMDS configuration. Pre-boot only.
         * @description Configures MMDS version, IPv4 address used by the MMDS network stack and interfaces that allow MMDS requests.
         */
        put: operations["putMmdsConfig"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/entropy": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /**
         * Creates an entropy device. Pre-boot only.
         * @description Enables an entropy device that provides high-quality random data to the guest.
         */
        put: operations["putEntropyDevice"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/serial": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /**
         * Configures the serial console
         * @description Configure the serial console, which the guest can write its kernel logs to. Has no effect if the serial console is not also enabled on the guest kernel command line
         */
        put: operations["putSerialDevice"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/hotplug/memory": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Retrieves the status of the hotpluggable memory
         * @description Reuturn the status of the hotpluggable memory. This can be used to follow the progress of the guest after a PATCH API.
         */
        get: operations["getMemoryHotplug"];
        /**
         * Configures the hotpluggable memory
         * @description Configure the hotpluggable memory, which is a virtio-mem device, with an associated memory area that can be hot(un)plugged in the guest on demand using the PATCH API.
         */
        put: operations["putMemoryHotplug"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /**
         * Updates the size of the hotpluggable memory region
         * @description Updates the size of the hotpluggable memory region. The guest will plug and unplug memory to hit the requested memory.
         */
        patch: operations["patchMemoryHotplug"];
        trace?: never;
    };
    "/network-interfaces/{iface_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /**
         * Creates a network interface. Pre-boot only.
         * @description Creates new network interface with ID specified by iface_id path parameter.
         */
        put: operations["putGuestNetworkInterfaceByID"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /**
         * Updates the rate limiters applied to a network interface. Post-boot only.
         * @description Updates the rate limiters applied to a network interface.
         */
        patch: operations["patchGuestNetworkInterfaceByID"];
        trace?: never;
    };
    "/snapshot/create": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /**
         * Creates a full or diff snapshot. Post-boot only.
         * @description Creates a snapshot of the microVM state. The microVM should be in the `Paused` state.
         */
        put: operations["createSnapshot"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/snapshot/load": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /**
         * Loads a snapshot. Pre-boot only.
         * @description Loads the microVM state from a snapshot. Only accepted on a fresh Firecracker process (before configuring any resource other than the Logger and Metrics).
         */
        put: operations["loadSnapshot"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/version": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** Gets the Firecracker version. */
        get: operations["getFirecrackerVersion"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/vm": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        /**
         * Updates the microVM state.
         * @description Sets the desired state (Paused or Resumed) for the microVM.
         */
        patch: operations["patchVm"];
        trace?: never;
    };
    "/vm/config": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Gets the full VM configuration.
         * @description Gets configuration for all VM resources. If the VM is restored from a snapshot, the boot-source, machine-config.smt and machine-config.cpu_template will be empty.
         */
        get: operations["getExportVmConfig"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/vsock": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /**
         * Creates/updates a vsock device. Pre-boot only.
         * @description The first call creates the device with the configuration specified in body. Subsequent calls will update the device configuration. May fail if update is not possible.
         */
        put: operations["putGuestVsock"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
}
export type webhooks = Record<string, never>;
export interface components {
  schemas: {
    /** @description Balloon device descriptor. */
    Balloon: {
      /** @description Target balloon size in MiB. */
      amount_mib: number;
      /** @description Whether the balloon should deflate when the guest has memory pressure. */
      deflate_on_oom: boolean;
      /** @description Interval in seconds between refreshing statistics. A non-zero value will enable the statistics. Defaults to 0. */
      stats_polling_interval_s?: number;
      /** @description Whether the free page hinting feature is enabled. */
      free_page_hinting?: boolean;
      /** @description Whether the free page reporting feature is enabled. */
      free_page_reporting?: boolean;
    };
    /** @description Balloon device descriptor. */
    BalloonUpdate: {
      /** @description Target balloon size in MiB. */
      amount_mib: number;
    };
    /** @description Describes the balloon device statistics. */
    BalloonStats: {
      /** @description Target number of pages the device aims to hold. */
      target_pages: number;
      /** @description Actual number of pages the device is holding. */
      actual_pages: number;
      /** @description Target amount of memory (in MiB) the device aims to hold. */
      target_mib: number;
      /** @description Actual amount of memory (in MiB) the device is holding. */
      actual_mib: number;
      /**
       * Format: int64
       * @description The amount of memory that has been swapped in (in bytes).
       */
      swap_in?: number;
      /**
       * Format: int64
       * @description The amount of memory that has been swapped out to disk (in bytes).
       */
      swap_out?: number;
      /**
       * Format: int64
       * @description The number of major page faults that have occurred.
       */
      major_faults?: number;
      /**
       * Format: int64
       * @description The number of minor page faults that have occurred.
       */
      minor_faults?: number;
      /**
       * Format: int64
       * @description The amount of memory not being used for any purpose (in bytes).
       */
      free_memory?: number;
      /**
       * Format: int64
       * @description The total amount of memory available (in bytes).
       */
      total_memory?: number;
      /**
       * Format: int64
       * @description An estimate of how much memory is available (in bytes) for starting new applications, without pushing the system to swap.
       */
      available_memory?: number;
      /**
       * Format: int64
       * @description The amount of memory, in bytes, that can be quickly reclaimed without additional I/O. Typically these pages are used for caching files from disk.
       */
      disk_caches?: number;
      /**
       * Format: int64
       * @description The number of successful hugetlb page allocations in the guest.
       */
      hugetlb_allocations?: number;
      /**
       * Format: int64
       * @description The number of failed hugetlb page allocations in the guest.
       */
      hugetlb_failures?: number;
      /**
       * Format: int64
       * @description OOM killer invocations, indicating critical memory pressure.
       */
      oom_kill?: number;
      /**
       * Format: int64
       * @description Counter of Allocation enter a slow path to gain more memory page. The reclaim/scan metrics can reveal what is actually happening.
       */
      alloc_stall?: number;
      /**
       * Format: int64
       * @description Amount of memory scanned asynchronously.
       */
      async_scan?: number;
      /**
       * Format: int64
       * @description Amount of memory scanned directly.
       */
      direct_scan?: number;
      /**
       * Format: int64
       * @description Amount of memory reclaimed asynchronously.
       */
      async_reclaim?: number;
      /**
       * Format: int64
       * @description Amount of memory reclaimed directly.
       */
      direct_reclaim?: number;
    };
    /** @description Command used to start a free page hinting run. */
    BalloonStartCmd: {
      /** @description If Firecracker should automatically acknowledge when the guest submits a done cmd. */
      acknowledge_on_stop?: boolean;
    };
    /** @description Describes the free page hinting status. */
    BalloonHintingStatus: {
      /** @description The last command issued by the host. */
      host_cmd: number;
      /** @description The last command provided by the guest. */
      guest_cmd?: number;
    };
    /** @description Update the statistics polling interval, with the first statistics update scheduled immediately. Statistics cannot be turned on/off after boot. */
    BalloonStatsUpdate: {
      /** @description Interval in seconds between refreshing statistics. */
      stats_polling_interval_s: number;
    };
    /** @description Boot source descriptor. */
    BootSource: {
      /** @description Kernel boot arguments */
      boot_args?: string;
      /** @description Host level path to the initrd image used to boot the guest */
      initrd_path?: string;
      /** @description Host level path to the kernel image used to boot the guest */
      kernel_image_path: string;
    };
    /**
     * @description The CPU Template defines a set of flags to be disabled from the microvm so that the features exposed to the guest are the same as in the selected instance type. This parameter has been deprecated and it will be removed in future Firecracker release.
     * @default None
     * @enum {string}
     */
    CpuTemplate: "C3" | "T2" | "T2S" | "T2CL" | "T2A" | "V1N1" | "None";
    /** @description The CPU configuration template defines a set of bit maps as modifiers of flags accessed by register to be disabled/enabled for the microvm. */
    CpuConfig: {
      /** @description A collection of KVM capabilities to be added or removed (both x86_64 and aarch64) */
      kvm_capabilities?: string[];
      /** @description A collection of CPUID leaf modifiers (x86_64 only) */
      cpuid_modifiers?: components["schemas"]["CpuidLeafModifier"][];
      /** @description A collection of model specific register modifiers (x86_64 only) */
      msr_modifiers?: components["schemas"]["MsrModifier"][];
      /** @description A collection of register modifiers (aarch64 only) */
      reg_modifiers?: components["schemas"]["ArmRegisterModifier"][];
      /** @description A collection of vCPU features to be modified (aarch64 only) */
      vcpu_features?: components["schemas"]["VcpuFeatures"][];
    };
    /** @description Modifier for a CPUID leaf and subleaf (x86_64) */
    CpuidLeafModifier: {
      /** @description CPUID leaf index as hex, binary, or decimal string (e.g., "0x0", "0b0", "0")) */
      leaf: string;
      /** @description CPUID subleaf index as hex, binary, or decimal string (e.g., "0x0", "0b0", "0") */
      subleaf: string;
      /**
       * Format: int32
       * @description KVM feature flags for this leaf-subleaf
       */
      flags: number;
      /** @description Register modifiers for this CPUID leaf */
      modifiers: components["schemas"]["CpuidRegisterModifier"][];
    };
    /** @description Modifier for a specific CPUID register within a leaf (x86_64) */
    CpuidRegisterModifier: {
      /**
       * @description Target CPUID register name
       * @enum {string}
       */
      register: "eax" | "ebx" | "ecx" | "edx";
      /** @description 32-bit bitmap string defining which bits to modify. Format is "0b" followed by 32 characters where '0' = clear bit, '1' = set bit, 'x' = don't modify. Example "0b00000000000000000000000000000001" or "0bxxxxxxxxxxxxxxxxxxxxxxxxxxxx0001" */
      bitmap: string;
    };
    /** @description Modifier for a model specific register (x86_64) */
    MsrModifier: {
      /** @description 32-bit MSR address as hex, binary, or decimal string (e.g., "0x10a", "0b100001010", "266") */
      addr: string;
      /** @description 64-bit bitmap string defining which bits to modify. Format is "0b" followed by 64 characters where '0' = clear bit, '1' = set bit, 'x' = don't modify. Underscores can be used for readability. Example "0b0000000000000000000000000000000000000000000000000000000000000001" */
      bitmap: string;
    };
    /** @description Modifier for an ARM register (aarch64) */
    ArmRegisterModifier: {
      /** @description 64-bit register address as hex, binary, or decimal string (e.g., "0x0", "0b0", "0") */
      addr: string;
      /** @description 128-bit bitmap string defining which bits to modify. Format is "0b" followed by up to 128 characters where '0' = clear bit, '1' = set bit, 'x' = don't modify. Underscores can be used for readability. Example "0b0000000000000000000000000000000000000000000000000000000000000001" */
      bitmap: string;
    };
    /** @description vCPU feature modifier (aarch64) */
    VcpuFeatures: {
      /**
       * Format: int32
       * @description Index in the kvm_vcpu_init.features array
       */
      index: number;
      /** @description 32-bit bitmap string defining which bits to modify. Format is "0b" followed by 32 characters where '0' = clear bit, '1' = set bit, 'x' = don't modify. Example "0b00000000000000000000000001100000" */
      bitmap: string;
    };
    Drive: {
      drive_id: string;
      /** @description Represents the unique id of the boot partition of this device. It is optional and it will be taken into account only if the is_root_device field is true. */
      partuuid?: string;
      is_root_device: boolean;
      /**
       * @description Represents the caching strategy for the block device.
       * @default Unsafe
       * @enum {string}
       */
      cache_type: "Unsafe" | "Writeback";
      /** @description Is block read only. This field is required for virtio-block config and should be omitted for vhost-user-block configuration. */
      is_read_only?: boolean;
      /** @description Host level path for the guest drive. This field is required for virtio-block config and should be omitted for vhost-user-block configuration. */
      path_on_host?: string;
      rate_limiter?: components["schemas"]["RateLimiter"];
      /**
       * @description Type of the IO engine used by the device. "Async" is supported on host kernels newer than 5.10.51. This field is optional for virtio-block config and should be omitted for vhost-user-block configuration.
       * @default Sync
       * @enum {string}
       */
      io_engine: "Sync" | "Async";
      /** @description Path to the socket of vhost-user-block backend. This field is required for vhost-user-block config should be omitted for virtio-block configuration. */
      socket?: string;
    };
    Pmem: {
      /** @description Identificator for this device. */
      id: string;
      /** @description Host level path for the virtio-pmem device to use as a backing file. */
      path_on_host: string;
      /** @description Flag to make this device be the root device for VM boot. Setting this flag will fail if there is another device configured to be a root device already. */
      root_device?: boolean;
      /** @description Flag to map backing file in read-only mode. */
      read_only?: boolean;
    };
    Error: {
      /** @description A description of the error condition */
      readonly fault_message?: string;
    };
    FullVmConfiguration: {
      balloon?: components["schemas"]["Balloon"];
      /** @description Configurations for all block devices. */
      drives?: components["schemas"]["Drive"][];
      "boot-source"?: components["schemas"]["BootSource"];
      "cpu-config"?: components["schemas"]["CpuConfig"];
      logger?: components["schemas"]["Logger"];
      "machine-config"?: components["schemas"]["MachineConfiguration"];
      metrics?: components["schemas"]["Metrics"];
      "memory-hotplug"?: components["schemas"]["MemoryHotplugConfig"];
      "mmds-config"?: components["schemas"]["MmdsConfig"];
      /** @description Configurations for all net devices. */
      "network-interfaces"?: components["schemas"]["NetworkInterface"][];
      /** @description Configurations for all pmem devices. */
      pmem?: components["schemas"]["Pmem"][];
      vsock?: components["schemas"]["Vsock"];
      entropy?: components["schemas"]["EntropyDevice"];
    };
    /** @description Variant wrapper containing the real action. */
    InstanceActionInfo: {
      /**
       * @description Enumeration indicating what type of action is contained in the payload
       * @enum {string}
       */
      action_type: "FlushMetrics" | "InstanceStart" | "SendCtrlAltDel";
    };
    /** @description Describes MicroVM instance information. */
    InstanceInfo: {
      /** @description Application name. */
      app_name: string;
      /** @description MicroVM / instance ID. */
      id: string;
      /**
       * @description The current detailed state (Not started, Running, Paused) of the Firecracker instance. This value is read-only for the control-plane.
       * @enum {string}
       */
      state: "Not started" | "Running" | "Paused";
      /** @description MicroVM hypervisor build version. */
      vmm_version: string;
    };
    /** @description Describes the configuration option for the logging capability. */
    Logger: {
      /**
       * @description Set the level. The possible values are case-insensitive.
       * @default Info
       * @enum {string}
       */
      level: "Error" | "Warning" | "Info" | "Debug" | "Trace" | "Off";
      /** @description Path to the named pipe or file for the human readable log output. */
      log_path?: string;
      /**
       * @description Whether or not to output the level in the logs.
       * @default false
       */
      show_level: boolean;
      /**
       * @description Whether or not to include the file path and line number of the log's origin.
       * @default false
       */
      show_log_origin: boolean;
      /**
       * @description The module path to filter log messages by.
       * @example api_server::request
       */
      module?: string;
    };
    /** @description Describes the number of vCPUs, memory size, SMT capabilities, huge page configuration and the CPU template. */
    MachineConfiguration: {
      cpu_template?: components["schemas"]["CpuTemplate"];
      /**
       * @description Flag for enabling/disabling simultaneous multithreading. Can be enabled only on x86.
       * @default false
       */
      smt: boolean;
      /** @description Memory size of VM */
      mem_size_mib: number;
      /**
       * @description Enable dirty page tracking. If this is enabled, then incremental guest memory snapshots can be created. These belong to diff snapshots, which contain, besides the microVM state, only the memory dirtied since a previous snapshot. Full snapshots each contain a full copy of the guest memory.
       * @default false
       */
      track_dirty_pages: boolean;
      /** @description Number of vCPUs (either 1 or an even number) */
      vcpu_count: number;
      /**
       * @description Which huge pages configuration (if any) should be used to back guest memory.
       * @enum {string}
       */
      huge_pages?: "None" | "2M";
    };
    MemoryBackend: {
      /** @enum {string} */
      backend_type: "File" | "Uffd";
      /** @description Based on 'backend_type' it is either 1) Path to the file that contains the guest memory to be loaded 2) Path to the UDS where a process is listening for a UFFD initialization control payload and open file descriptor that it can use to serve this process's guest memory page faults */
      backend_path: string;
    };
    /** @description Describes the configuration option for the metrics capability. */
    Metrics: {
      /** @description Path to the named pipe or file where the JSON-formatted metrics are flushed. */
      metrics_path: string;
    };
    /** @description Defines the MMDS configuration. */
    MmdsConfig: {
      /**
       * @description Enumeration indicating the MMDS version to be configured.
       * @default V1
       * @enum {string}
       */
      version: "V1" | "V2";
      /** @description List of the network interface IDs capable of forwarding packets to the MMDS. Network interface IDs mentioned must be valid at the time of this request. The net device model will reply to HTTP GET requests sent to the MMDS address via the interfaces mentioned. In this case, both ARP requests and TCP segments heading to `ipv4_address` are intercepted by the device model, and do not reach the associated TAP device. */
      network_interfaces: string[];
      /**
       * Format: 169.254.([1-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-4]).([0-9]|[1-9][0-9]|1[0-9][0-9]|2[0-4][0-9]|25[0-5])
       * @description A valid IPv4 link-local address.
       * @default 169.254.169.254
       */
      ipv4_address: string;
      /**
       * @description MMDS operates compatibly with EC2 IMDS (i.e. responds "text/plain" content regardless of Accept header in requests).
       * @default false
       */
      imds_compat: boolean;
    };
    /** @description Describes the contents of MMDS in JSON format. */
    MmdsContentsObject: {
      [key: string]: unknown;
    };
    /** @description Defines a network interface. */
    NetworkInterface: {
      guest_mac?: string;
      /** @description Host level path for the guest network interface */
      host_dev_name: string;
      iface_id: string;
      rx_rate_limiter?: components["schemas"]["RateLimiter"];
      tx_rate_limiter?: components["schemas"]["RateLimiter"];
    };
    PartialDrive: {
      drive_id: string;
      /** @description Host level path for the guest drive. This field is optional for virtio-block config and should be omitted for vhost-user-block configuration. */
      path_on_host?: string;
      rate_limiter?: components["schemas"]["RateLimiter"];
    };
    /** @description Defines a partial network interface structure, used to update the rate limiters for that interface, after microvm start. */
    PartialNetworkInterface: {
      iface_id: string;
      rx_rate_limiter?: components["schemas"]["RateLimiter"];
      tx_rate_limiter?: components["schemas"]["RateLimiter"];
    };
    /** @description Defines an IO rate limiter with independent bytes/s and ops/s limits. Limits are defined by configuring each of the _bandwidth_ and _ops_ token buckets. This field is optional for virtio-block config and should be omitted for vhost-user-block configuration. */
    RateLimiter: {
      bandwidth?: components["schemas"]["TokenBucket"];
      ops?: components["schemas"]["TokenBucket"];
    };
    SnapshotCreateParams: {
      /** @description Path to the file that will contain the guest memory. */
      mem_file_path: string;
      /** @description Path to the file that will contain the microVM state. */
      snapshot_path: string;
      /**
       * @description Type of snapshot to create. It is optional and by default, a full snapshot is created.
       * @enum {string}
       */
      snapshot_type?: "Full" | "Diff";
    };
    /** @description Allows for changing the backing TAP device of a network interface during snapshot restore. */
    NetworkOverride: {
      /** @description The name of the interface to modify */
      iface_id: string;
      /** @description The new host device of the interface */
      host_dev_name: string;
    };
    /** @description Defines the configuration used for handling snapshot resume. Exactly one of the two `mem_*` fields must be present in the body of the request. */
    SnapshotLoadParams: {
      /** @description (Deprecated) Enable dirty page tracking to improve space efficiency of diff snapshots */
      enable_diff_snapshots?: boolean;
      /** @description Enable dirty page tracking to improve space efficiency of diff snapshots */
      track_dirty_pages?: boolean;
      /** @description Path to the file that contains the guest memory to be loaded. It is only allowed if `mem_backend` is not present. This parameter has been deprecated and it will be removed in future Firecracker release. */
      mem_file_path?: string;
      mem_backend?: components["schemas"]["MemoryBackend"];
      /** @description Path to the file that contains the microVM state to be loaded. */
      snapshot_path: string;
      /** @description When set to true, the vm is also resumed if the snapshot load is successful. */
      resume_vm?: boolean;
      /** @description Network host device names to override */
      network_overrides?: components["schemas"]["NetworkOverride"][];
    };
    /** @description Defines a token bucket with a maximum capacity (size), an initial burst size (one_time_burst) and an interval for refilling purposes (refill_time). The refill-rate is derived from size and refill_time, and it is the constant rate at which the tokens replenish. The refill process only starts happening after the initial burst budget is consumed. Consumption from the token bucket is unbounded in speed which allows for bursts bound in size by the amount of tokens available. Once the token bucket is empty, consumption speed is bound by the refill_rate. */
    TokenBucket: {
      /**
       * Format: int64
       * @description The initial size of a token bucket.
       */
      one_time_burst?: number;
      /**
       * Format: int64
       * @description The amount of milliseconds it takes for the bucket to refill.
       */
      refill_time: number;
      /**
       * Format: int64
       * @description The total number of tokens this bucket can hold.
       */
      size: number;
    };
    /** @description Defines the microVM running state. It is especially useful in the snapshotting context. */
    Vm: {
      /** @enum {string} */
      state: "Paused" | "Resumed";
    };
    /** @description Defines an entropy device. */
    EntropyDevice: {
      rate_limiter?: components["schemas"]["RateLimiter"];
    };
    /** @description The configuration of the serial device */
    SerialDevice: {
      /** @description Path to a file or named pipe on the host to which serial output should be written. */
      serial_out_path?: string;
    };
    /** @description The configuration of the hotpluggable memory device (virtio-mem) */
    MemoryHotplugConfig: {
      /** @description Total size of the hotpluggable memory in MiB. */
      total_size_mib?: number;
      /**
       * @description Slot size for the hotpluggable memory in MiB. This will determine the granularity of hot-plug memory from the host. Refer to the device documentation on how to tune this value.
       * @default 128
       */
      slot_size_mib: number;
      /**
       * @description (Logical) Block size for the hotpluggable memory in MiB. This will determine the logical granularity of hot-plug memory for the guest. Refer to the device documentation on how to tune this value.
       * @default 2
       */
      block_size_mib: number;
    };
    /** @description An update to the size of the hotpluggable memory region. */
    MemoryHotplugSizeUpdate: {
      /** @description New target region size. */
      requested_size_mib?: number;
    };
    /** @description The status of the hotpluggable memory device (virtio-mem) */
    MemoryHotplugStatus: {
      /** @description Total size of the hotpluggable memory in MiB. */
      total_size_mib?: number;
      /** @description Slot size for the hotpluggable memory in MiB. */
      slot_size_mib?: number;
      /** @description (Logical) Block size for the hotpluggable memory in MiB. */
      block_size_mib?: number;
      /** @description Plugged size for the hotpluggable memory in MiB. */
      plugged_size_mib?: number;
      /** @description Requested size for the hotpluggable memory in MiB. */
      requested_size_mib?: number;
    };
    /** @description Describes the Firecracker version. */
    FirecrackerVersion: {
      /** @description Firecracker build version. */
      firecracker_version: string;
    };
    /** @description Defines a vsock device, backed by a set of Unix Domain Sockets, on the host side. For host-initiated connections, Firecracker will be listening on the Unix socket identified by the path `uds_path`. Firecracker will create this socket, bind and listen on it. Host-initiated connections will be performed by connection to this socket and issuing a connection forwarding request to the desired guest-side vsock port (i.e. `CONNECT 52\n`, to connect to port 52). For guest-initiated connections, Firecracker will expect host software to be bound and listening on Unix sockets at `uds_path_<PORT>`. E.g. "/path/to/host_vsock.sock_52" for port number 52. */
    Vsock: {
      /** @description Guest Vsock CID */
      guest_cid: number;
      /** @description Path to UNIX domain socket, used to proxy vsock connections. */
      uds_path: string;
      /** @description This parameter has been deprecated and it will be removed in future Firecracker release. */
      vsock_id?: string;
    };
  };
  responses: never;
  parameters: never;
  requestBodies: never;
  headers: never;
  pathItems: never;
}
export type $defs = Record<string, never>;
export interface operations {
  describeInstance: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description The instance information */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["InstanceInfo"];
        };
      };
      /** @description Internal Server Error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  createSyncAction: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    requestBody: {
      content: {
        "application/json": components["schemas"]["InstanceActionInfo"];
      };
    };
    responses: {
      /** @description The update was successful */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description The action cannot be executed due to bad input */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal Server Error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  describeBalloonConfig: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description The balloon device configuration */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Balloon"];
        };
      };
      /** @description Balloon device not configured. */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal Server Error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  putBalloon: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description Balloon properties */
    requestBody: {
      content: {
        "application/json": components["schemas"]["Balloon"];
      };
    };
    responses: {
      /** @description Balloon device created/updated */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Balloon device cannot be created/updated due to bad input */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  patchBalloon: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description Balloon properties */
    requestBody: {
      content: {
        "application/json": components["schemas"]["BalloonUpdate"];
      };
    };
    responses: {
      /** @description Balloon device updated */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Balloon device cannot be updated due to bad input */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  describeBalloonStats: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description The balloon device statistics */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["BalloonStats"];
        };
      };
      /** @description The balloon device statistics were not enabled when the device was configured. */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal Server Error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  patchBalloonStatsInterval: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description Balloon properties */
    requestBody: {
      content: {
        "application/json": components["schemas"]["BalloonStatsUpdate"];
      };
    };
    responses: {
      /** @description Balloon statistics interval updated */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Balloon statistics interval cannot be updated due to bad input */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  startBalloonHinting: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description When the device completes the hinting whether we should automatically ack this. */
    requestBody?: {
      content: {
        "application/json": components["schemas"]["BalloonStartCmd"];
      };
    };
    responses: {
      /** @description Free page hinting run started. */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description The balloon free hinting was not enabled when the device was configured. */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal Server Error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  describeBalloonHinting: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description The balloon free page hinting statistics */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["BalloonHintingStatus"];
        };
      };
      /** @description The balloon free hinting was not enabled when the device was configured. */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal Server Error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  stopBalloonHinting: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description Free page hinting run stopped. */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description The balloon free hinting was not enabled when the device was configured. */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal Server Error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  putGuestBootSource: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description Guest boot source properties */
    requestBody: {
      content: {
        "application/json": components["schemas"]["BootSource"];
      };
    };
    responses: {
      /** @description Boot source created/updated */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Boot source cannot be created due to bad input */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  putCpuConfiguration: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description CPU configuration request */
    requestBody?: {
      content: {
        "application/json": components["schemas"]["CpuConfig"];
      };
    };
    responses: {
      /** @description CPU configuration set successfully */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description CPU configuration cannot be updated due to invalid input format */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  putGuestDriveByID: {
    parameters: {
      query?: never;
      header?: never;
      path: {
        /** @description The id of the guest drive */
        drive_id: string;
      };
      cookie?: never;
    };
    /** @description Guest drive properties */
    requestBody: {
      content: {
        "application/json": components["schemas"]["Drive"];
      };
    };
    responses: {
      /** @description Drive created/updated */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Drive cannot be created/updated due to bad input */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error. */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  patchGuestDriveByID: {
    parameters: {
      query?: never;
      header?: never;
      path: {
        /** @description The id of the guest drive */
        drive_id: string;
      };
      cookie?: never;
    };
    /** @description Guest drive properties */
    requestBody: {
      content: {
        "application/json": components["schemas"]["PartialDrive"];
      };
    };
    responses: {
      /** @description Drive updated */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Drive cannot be updated due to bad input */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error. */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  putGuestPmemByID: {
    parameters: {
      query?: never;
      header?: never;
      path: {
        /** @description The id of the guest pmem device */
        id: string;
      };
      cookie?: never;
    };
    /** @description Guest pmem device properties */
    requestBody: {
      content: {
        "application/json": components["schemas"]["Pmem"];
      };
    };
    responses: {
      /** @description Pmem device is created/updated */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Pmem device cannot be created/updated due to bad input */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error. */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  putLogger: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description Logging system description */
    requestBody: {
      content: {
        "application/json": components["schemas"]["Logger"];
      };
    };
    responses: {
      /** @description Logger created. */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Logger cannot be initialized due to bad input. */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error. */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  getMachineConfiguration: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description OK */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["MachineConfiguration"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  putMachineConfiguration: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description Machine Configuration Parameters */
    requestBody?: {
      content: {
        "application/json": components["schemas"]["MachineConfiguration"];
      };
    };
    responses: {
      /** @description Machine Configuration created/updated */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Machine Configuration cannot be updated due to bad input */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  patchMachineConfiguration: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description A subset of Machine Configuration Parameters */
    requestBody?: {
      content: {
        "application/json": components["schemas"]["MachineConfiguration"];
      };
    };
    responses: {
      /** @description Machine Configuration created/updated */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Machine Configuration cannot be updated due to bad input */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  putMetrics: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description Metrics system description */
    requestBody: {
      content: {
        "application/json": components["schemas"]["Metrics"];
      };
    };
    responses: {
      /** @description Metrics system created. */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Metrics system cannot be initialized due to bad input request or metrics system already initialized. */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error. */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  getMmds: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description The MMDS data store JSON. */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": {
            [key: string]: unknown;
          };
        };
      };
      /** @description The MMDS data store content can not be found. */
      404: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  putMmds: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description The MMDS data store as JSON. */
    requestBody?: {
      content: {
        "application/json": components["schemas"]["MmdsContentsObject"];
      };
    };
    responses: {
      /** @description MMDS data store created/updated. */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description MMDS data store cannot be created due to bad input. */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  patchMmds: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description The MMDS data store patch JSON. */
    requestBody?: {
      content: {
        "application/json": components["schemas"]["MmdsContentsObject"];
      };
    };
    responses: {
      /** @description MMDS data store updated. */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description MMDS data store cannot be updated due to bad input. */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  putMmdsConfig: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description The MMDS configuration as JSON. */
    requestBody: {
      content: {
        "application/json": components["schemas"]["MmdsConfig"];
      };
    };
    responses: {
      /** @description MMDS configuration was created/updated. */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description MMDS configuration cannot be updated due to bad input. */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  putEntropyDevice: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description Guest entropy device properties */
    requestBody: {
      content: {
        "application/json": components["schemas"]["EntropyDevice"];
      };
    };
    responses: {
      /** @description Entropy device created */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  putSerialDevice: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description Serial console properties */
    requestBody: {
      content: {
        "application/json": components["schemas"]["SerialDevice"];
      };
    };
    responses: {
      /** @description Serial device configured */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  getMemoryHotplug: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description OK */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["MemoryHotplugStatus"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  putMemoryHotplug: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description Hotpluggable memory configuration */
    requestBody: {
      content: {
        "application/json": components["schemas"]["MemoryHotplugConfig"];
      };
    };
    responses: {
      /** @description Hotpluggable memory configured */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  patchMemoryHotplug: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description Hotpluggable memory size update */
    requestBody: {
      content: {
        "application/json": components["schemas"]["MemoryHotplugSizeUpdate"];
      };
    };
    responses: {
      /** @description Hotpluggable memory configured */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  putGuestNetworkInterfaceByID: {
    parameters: {
      query?: never;
      header?: never;
      path: {
        /** @description The id of the guest network interface */
        iface_id: string;
      };
      cookie?: never;
    };
    /** @description Guest network interface properties */
    requestBody: {
      content: {
        "application/json": components["schemas"]["NetworkInterface"];
      };
    };
    responses: {
      /** @description Network interface created/updated */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Network interface cannot be created due to bad input */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  patchGuestNetworkInterfaceByID: {
    parameters: {
      query?: never;
      header?: never;
      path: {
        /** @description The id of the guest network interface */
        iface_id: string;
      };
      cookie?: never;
    };
    /** @description A subset of the guest network interface properties */
    requestBody: {
      content: {
        "application/json": components["schemas"]["PartialNetworkInterface"];
      };
    };
    responses: {
      /** @description Network interface updated */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Network interface cannot be updated due to bad input */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  createSnapshot: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description The configuration used for creating a snapshot. */
    requestBody: {
      content: {
        "application/json": components["schemas"]["SnapshotCreateParams"];
      };
    };
    responses: {
      /** @description Snapshot created */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Snapshot cannot be created due to bad input */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  loadSnapshot: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description The configuration used for loading a snapshot. */
    requestBody: {
      content: {
        "application/json": components["schemas"]["SnapshotLoadParams"];
      };
    };
    responses: {
      /** @description Snapshot loaded */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Snapshot cannot be loaded due to bad input */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  getFirecrackerVersion: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description OK */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["FirecrackerVersion"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  patchVm: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description The microVM state */
    requestBody: {
      content: {
        "application/json": components["schemas"]["Vm"];
      };
    };
    responses: {
      /** @description Vm state updated */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Vm state cannot be updated due to bad input */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  getExportVmConfig: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    requestBody?: never;
    responses: {
      /** @description OK */
      200: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["FullVmConfiguration"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
  putGuestVsock: {
    parameters: {
      query?: never;
      header?: never;
      path?: never;
      cookie?: never;
    };
    /** @description Guest vsock properties */
    requestBody: {
      content: {
        "application/json": components["schemas"]["Vsock"];
      };
    };
    responses: {
      /** @description Vsock created/updated */
      204: {
        headers: {
          [name: string]: unknown;
        };
        content?: never;
      };
      /** @description Vsock cannot be created due to bad input */
      400: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
      /** @description Internal server error */
      default: {
        headers: {
          [name: string]: unknown;
        };
        content: {
          "application/json": components["schemas"]["Error"];
        };
      };
    };
  };
}
