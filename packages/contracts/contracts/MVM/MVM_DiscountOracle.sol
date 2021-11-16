// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;
/* Contract Imports */
/* External Imports */

import { iMVM_DiscountOracle } from "./iMVM_DiscountOracle.sol";
import { Lib_AddressResolver } from "../libraries/resolver/Lib_AddressResolver.sol";

contract MVM_DiscountOracle is iMVM_DiscountOracle, Lib_AddressResolver{
    // Current l2 gas price
    uint256 public discount;
    uint256 public minL2Gas;
    mapping (address => bool) public xDomainWL;
    bool allowAllXDomainSenders;
    string constant public CONFIG_OWNER_KEY = "METIS_MANAGER";
    
    /**********************
     * Function Modifiers *
     **********************/

    modifier onlyManager() {
        require(
            msg.sender == resolve(CONFIG_OWNER_KEY),
            "MVM_DiscountOracle: Function can only be called by the METIS_MANAGER."
        );
        _;
    }
    
    
    constructor(
      address _addressManager,
      uint256 _initialDiscount
    )
      Lib_AddressResolver(_addressManager)
    {
      setDiscount(_initialDiscount);
      setMinL2Gas(400_000);
      allowAllXDomainSenders = false;
    }
    
    
    function getMinL2Gas() view public override returns (uint256){
      return minL2Gas;
    }
    
    function getDiscount() view public override returns (uint256){
      return discount;
    }
    
    function setDiscount(
        uint256 _discount
    )
        public
        override
        onlyManager
    {
        discount = _discount;
    }
    
    function setMinL2Gas(
        uint256 _minL2Gas
    )
        public
        override
        onlyManager
    {
        minL2Gas = _minL2Gas;
    }
    
    function setWhitelistedXDomainSender(
        address _sender,
        bool _isWhitelisted
    )
        external
        override
        onlyManager
    {
        xDomainWL[_sender] = _isWhitelisted;
    }
    
    function isXDomainSenderAllowed(
        address _sender
    )
        view
        override
        public
        returns (
            bool
        )
    {
        return (
            allowAllXDomainSenders == true
            || xDomainWL[_sender]
        );
    }
    
    function setAllowAllXDomainSenders(
        bool _allowAllXDomainSenders
    )
        public
        override
        onlyManager
    {
        allowAllXDomainSenders = _allowAllXDomainSenders;
    }
    
    function processL2SeqGas(address sender, uint256 _chainId) 
    public payable override {
        require(isXDomainSenderAllowed(sender), "sender is not whitelisted");
        string memory ch = string(abi.encodePacked(uint2str(_chainId),"_MVM_Sequencer"));
        
        address sequencer = resolve(ch);
        require (sequencer != address(0), string(abi.encodePacked("sequencer address not available: ", ch)));
        
        //take the fee
        (bool sent, ) = sequencer.call{value: msg.value}("");
        require(sent, "Failed to send Ether");
    }
    

    function uint2str(uint _i) internal pure returns (string memory _uintAsString) {
        if (_i == 0) {
            return "0";
        }
        uint j = _i;
        uint len;
        while (j != 0) {
            len++;
            j /= 10;
        }
        bytes memory bstr = new bytes(len);
        uint k = len;
        while (_i != 0) {
            k = k-1;
            uint8 temp = (48 + uint8(_i - _i / 10 * 10));
            bytes1 b1 = bytes1(temp);
            bstr[k] = b1;
            _i /= 10;
        }
        return string(bstr);
    }
}
